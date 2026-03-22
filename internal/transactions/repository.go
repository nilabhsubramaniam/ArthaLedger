package transactions

import (
	"context"
	"math"

	"gorm.io/gorm"
)

// ── Interface ─────────────────────────────────────────────────────────────────

// Repository is the data-access contract for the transactions domain.
// Every query is scoped by userID so the SQL layer enforces row-level ownership.
type Repository interface {
	// Create inserts a single transaction row (used for income and expense).
	// Must be called inside a DB transaction (tx) so the balance update and the
	// row insert are committed or rolled back atomically.
	Create(ctx context.Context, tx *gorm.DB, t *Transaction) error

	// CreatePair inserts two transaction rows atomically (used for transfers).
	// Both rows are created inside the provided DB transaction.
	CreatePair(ctx context.Context, tx *gorm.DB, source, dest *Transaction) error

	// FindByIDAndUserID returns a single transaction owned by userID.
	// Returns gorm.ErrRecordNotFound when the ID does not exist or belongs to
	// another user — the service maps this to ErrTransactionNotFound.
	FindByIDAndUserID(ctx context.Context, id, userID uint) (*Transaction, error)

	// List returns a paginated, filtered slice of transactions for userID.
	// The filter is applied in SQL (not in memory) for efficiency.
	List(ctx context.Context, userID uint, f TransactionFilter) ([]Transaction, int64, error)

	// Update applies a map of field changes to one row (scoped to userID).
	Update(ctx context.Context, tx *gorm.DB, id, userID uint, updates map[string]interface{}) error

	// Delete soft-deletes a single transaction row (scoped to userID).
	Delete(ctx context.Context, tx *gorm.DB, id, userID uint) error

	// FindLinkedTransfer returns the other leg of a transfer pair.
	// Returns nil (no error) when the transaction has no linked leg.
	FindLinkedTransfer(ctx context.Context, refID uint) (*Transaction, error)

	// DB exposes the underlying *gorm.DB so the service can begin a DB
	// transaction that wraps both the transaction row and the balance update.
	DB() *gorm.DB
}

// ── PostgreSQL implementation ─────────────────────────────────────────────────

type postgresRepository struct {
	db *gorm.DB
}

// NewRepository returns a Repository backed by the given GORM handle.
func NewRepository(db *gorm.DB) Repository {
	return &postgresRepository{db: db}
}

// DB exposes the raw connection for beginning cross-table DB transactions.
func (r *postgresRepository) DB() *gorm.DB {
	return r.db
}

// Create inserts a single transaction row inside an existing DB transaction (tx).
// After this call, GORM populates t.ID, t.CreatedAt, t.UpdatedAt.
func (r *postgresRepository) Create(ctx context.Context, tx *gorm.DB, t *Transaction) error {
	return tx.WithContext(ctx).Create(t).Error
}

// CreatePair inserts two transaction rows inside the same DB transaction.
// Used exclusively for transfer operations — both legs must succeed together.
func (r *postgresRepository) CreatePair(ctx context.Context, tx *gorm.DB, source, dest *Transaction) error {
	// Insert the source leg first. After Create, source.ID is populated.
	if err := tx.WithContext(ctx).Create(source).Error; err != nil {
		return err
	}
	// Insert the destination leg; link it back to source.
	dest.TransferReferenceID = &source.ID
	if err := tx.WithContext(ctx).Create(dest).Error; err != nil {
		return err
	}
	// Now update source to point at dest (bidirectional link).
	source.TransferReferenceID = &dest.ID
	return tx.WithContext(ctx).
		Model(source).
		UpdateColumn("transfer_reference_id", dest.ID).Error
}

// FindByIDAndUserID returns the transaction matching both id and user_id.
// WHERE includes user_id so a user can never fetch another user's record.
func (r *postgresRepository) FindByIDAndUserID(ctx context.Context, id, userID uint) (*Transaction, error) {
	var t Transaction
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// List applies every non-nil filter field to the query, then paginates results.
// All filtering happens in the database — no in-memory post-processing.
func (r *postgresRepository) List(ctx context.Context, userID uint, f TransactionFilter) ([]Transaction, int64, error) {
	// Start a base query scoped to the user.
	q := r.db.WithContext(ctx).
		Model(&Transaction{}).
		Where("user_id = ?", userID)

	// Apply optional filters.
	if f.AccountID != nil {
		q = q.Where("account_id = ?", *f.AccountID)
	}
	if f.CategoryID != nil {
		q = q.Where("category_id = ?", *f.CategoryID)
	}
	if f.Type != "" {
		q = q.Where("type = ?", f.Type)
	}
	if f.DateFrom != nil {
		q = q.Where("date >= ?", *f.DateFrom)
	}
	if f.DateTo != nil {
		q = q.Where("date <= ?", *f.DateTo)
	}
	if f.MinAmount != nil {
		q = q.Where("amount >= ?", *f.MinAmount)
	}
	if f.MaxAmount != nil {
		q = q.Where("amount <= ?", *f.MaxAmount)
	}

	// Count matches before applying LIMIT / OFFSET.
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination. Clamp limit to [1, 100].
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	var txs []Transaction
	err := q.
		Order("date DESC, created_at DESC"). // newest transaction date first
		Limit(limit).
		Offset(offset).
		Find(&txs).Error
	if err != nil {
		return nil, 0, err
	}

	return txs, total, nil
}

// Update applies field-level changes within an existing DB transaction.
// Only the keys present in the updates map are modified (no zero-value overwriting).
func (r *postgresRepository) Update(ctx context.Context, tx *gorm.DB, id, userID uint, updates map[string]interface{}) error {
	result := tx.WithContext(ctx).
		Model(&Transaction{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete soft-deletes a single transaction row within the provided DB transaction.
func (r *postgresRepository) Delete(ctx context.Context, tx *gorm.DB, id, userID uint) error {
	result := tx.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&Transaction{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// FindLinkedTransfer finds the other leg of a transfer pair using
// transfer_reference_id. Returns (nil, nil) when there is no linked row.
func (r *postgresRepository) FindLinkedTransfer(ctx context.Context, refID uint) (*Transaction, error) {
	var t Transaction
	err := r.db.WithContext(ctx).
		Where("id = ?", refID).
		First(&t).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // no linked leg — not an error
		}
		return nil, err
	}
	return &t, nil
}

// ── Pagination helper (shared with service) ───────────────────────────────────

// BuildPagination constructs the Pagination metadata returned in list responses.
// Called by the service after the repository returns total and limit.
func BuildPagination(page, limit int, total int64) Pagination {
	if limit <= 0 {
		limit = 20
	}
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if totalPages < 1 {
		totalPages = 1
	}
	return Pagination{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
