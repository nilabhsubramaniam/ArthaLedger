package accounts

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ── Interface ──────────────────────────────────────────────────────────────────

// Repository is the data-access contract for the accounts package.
// Every method requires both a context (for cancellation / tracing) and the
// ownerID so the repository can enforce row-level ownership at the SQL layer —
// preventing any user from touching another user's data even if they guess an ID.
type Repository interface {
	// Create inserts a new account row and populates the struct with generated fields.
	Create(ctx context.Context, account *Account) error

	// FindByIDAndUserID returns the account matching both id and user_id.
	// Returns gorm.ErrRecordNotFound when the account does not exist or belongs
	// to a different user — callers map this to 404 (no information leakage).
	FindByIDAndUserID(ctx context.Context, id, userID uint) (*Account, error)

	// ListByUserID returns all non-deleted accounts for the given user.
	// Also returns the total count (same query, included for pagination readiness).
	ListByUserID(ctx context.Context, userID uint) ([]Account, int64, error)

	// Update persists changes to a specific account row (scoped to userID).
	// Only the fields present in the updates map are touched (GORM selective update).
	Update(ctx context.Context, id, userID uint, updates map[string]interface{}) error

	// Delete soft-deletes the account by setting deleted_at = NOW() (scoped to userID).
	Delete(ctx context.Context, id, userID uint) error

	// UpdateBalance modifies the account balance by the given delta inside an
	// existing DB transaction (tx). Positive delta = credit, negative = debit.
	// Must operate within a transaction so the balance and the transaction row
	// are committed or rolled back atomically.
	UpdateBalance(ctx context.Context, tx *gorm.DB, accountID uint, delta float64) error

	// HasTransactions returns true when the account has at least one non-deleted
	// transaction row. Used to guard against deleting accounts with history.
	HasTransactions(ctx context.Context, accountID uint) (bool, error)

	// GetSummary returns derived statistics for a single account.
	GetSummary(ctx context.Context, id, userID uint) (*AccountSummaryResponse, error)

	// DB exposes the raw *gorm.DB so the transaction service can begin a DB
	// transaction that spans both account-balance updates and transaction inserts.
	DB() *gorm.DB
}

// ── PostgreSQL implementation ──────────────────────────────────────────────────

type postgresRepository struct {
	db *gorm.DB
}

// NewRepository returns a Repository backed by the provided GORM handle.
func NewRepository(db *gorm.DB) Repository {
	return &postgresRepository{db: db}
}

// DB returns the underlying *gorm.DB for use in cross-package DB transactions.
func (r *postgresRepository) DB() *gorm.DB {
	return r.db
}

// Create inserts a new account.
// GORM populates ID, CreatedAt, UpdatedAt after the INSERT.
func (r *postgresRepository) Create(ctx context.Context, account *Account) error {
	return r.db.WithContext(ctx).Create(account).Error
}

// FindByIDAndUserID returns one account visible to the user.
// The WHERE clause includes both id AND user_id — a user cannot retrieve
// another user's account even if they know its ID.
func (r *postgresRepository) FindByIDAndUserID(ctx context.Context, id, userID uint) (*Account, error) {
	var account Account
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// ListByUserID returns all active (non-deleted) accounts for a user, newest first.
// It runs a COUNT(*) in the same call for the list response total.
func (r *postgresRepository) ListByUserID(ctx context.Context, userID uint) ([]Account, int64, error) {
	var accounts []Account
	var total int64

	// COUNT query — runs without ORDER BY for efficiency
	if err := r.db.WithContext(ctx).
		Model(&Account{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Fetch rows ordered by creation time descending (most recently created first)
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&accounts).Error; err != nil {
		return nil, 0, err
	}

	return accounts, total, nil
}

// Update uses GORM's map-based update so only the explicitly provided fields
// are changed and the UpdatedAt timestamp is refreshed automatically.
func (r *postgresRepository) Update(ctx context.Context, id, userID uint, updates map[string]interface{}) error {
	result := r.db.WithContext(ctx).
		Model(&Account{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	// RowsAffected == 0 means no row matched — either wrong id or wrong user.
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete sets deleted_at = NOW() (soft-delete). The row remains in the DB and
// can be recoverd manually, but GORM excludes it from all future queries.
func (r *postgresRepository) Delete(ctx context.Context, id, userID uint) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&Account{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateBalance adds delta to the current balance using a SQL expression rather
// than a read-then-write to prevent lost-update races in concurrent requests.
// Must be called inside a *gorm.DB transaction (tx) for atomicity with the
// related transaction row insert/delete.
func (r *postgresRepository) UpdateBalance(ctx context.Context, tx *gorm.DB, accountID uint, delta float64) error {
	// gorm.Expr generates: UPDATE accounts SET balance = balance + ?, updated_at = NOW() WHERE id = ?
	return tx.WithContext(ctx).
		Model(&Account{}).
		Where("id = ?", accountID).
		UpdateColumn("balance", gorm.Expr("balance + ?", delta)).Error
}

// HasTransactions counts non-deleted transaction rows for this account.
// Returns true if count > 0, so the caller can block the account deletion.
func (r *postgresRepository) HasTransactions(ctx context.Context, accountID uint) (bool, error) {
	var count int64
	// We reference the transactions table by name because importing the
	// transactions package would create a circular dependency.
	err := r.db.WithContext(ctx).
		Table("transactions").
		Where("account_id = ? AND deleted_at IS NULL", accountID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetSummary runs a single JOIN query to fetch account details plus aggregated
// transaction statistics in one round-trip.
func (r *postgresRepository) GetSummary(ctx context.Context, id, userID uint) (*AccountSummaryResponse, error) {
	// Fetch the account first — ownership check is baked into the WHERE clause.
	account, err := r.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		return nil, err
	}

	// Aggregate stats from the transactions table via a raw sub-query.
	type stats struct {
		TransactionCount    int64      `gorm:"column:cnt"`
		LastTransactionDate *time.Time `gorm:"column:last_date"`
	}
	var s stats
	r.db.WithContext(ctx).
		Table("transactions").
		Select("COUNT(*) AS cnt, MAX(date) AS last_date").
		Where("account_id = ? AND deleted_at IS NULL", id).
		Scan(&s)

	return &AccountSummaryResponse{
		Account:             toAccountResponse(account),
		TransactionCount:    s.TransactionCount,
		LastTransactionDate: s.LastTransactionDate,
	}, nil
}
