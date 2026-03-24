package budgets

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ── Interface ──────────────────────────────────────────────────────────────────

// Repository is the data-access contract for the budgets package.
// Every method requires a context for cancellation / tracing.
// Row-level ownership is enforced at the SQL layer (user_id in WHERE clause)
// so no user can read or modify another user's budgets even by guessing IDs.
type Repository interface {
	// Create inserts a new budget row and populates generated fields.
	Create(ctx context.Context, budget *Budget) error

	// FindByIDAndUserID returns the budget matching both id and user_id.
	// Returns gorm.ErrRecordNotFound when absent or owned by another user —
	// callers map this to 404 (no information leakage).
	FindByIDAndUserID(ctx context.Context, id, userID uint) (*Budget, error)

	// ListByUserID returns all non-deleted budgets for a user, newest first.
	// Also returns the total count for list-response metadata.
	ListByUserID(ctx context.Context, userID uint) ([]Budget, int64, error)

	// Update applies partial field changes using a map (GORM selective update).
	// Only the keys present in updates are written; UpdatedAt is refreshed automatically.
	Update(ctx context.Context, id, userID uint, updates map[string]interface{}) error

	// Delete soft-deletes a budget by setting deleted_at = NOW().
	Delete(ctx context.Context, id, userID uint) error

	// GetSpentAmount sums expense transactions in [from, to] for a user,
	// optionally filtered to a single category (nil = all categories).
	// Queries the transactions table directly so this package never imports
	// the transactions package, avoiding a circular dependency.
	GetSpentAmount(ctx context.Context, userID uint, categoryID *uint, from, to time.Time) (float64, error)
}

// ── PostgreSQL implementation ──────────────────────────────────────────────────

type postgresRepository struct {
	db *gorm.DB
}

// NewRepository returns a budgets Repository backed by the given GORM handle.
func NewRepository(db *gorm.DB) Repository {
	return &postgresRepository{db: db}
}

// Create inserts the budget and lets PostgreSQL/GORM populate ID and timestamps.
func (r *postgresRepository) Create(ctx context.Context, budget *Budget) error {
	return r.db.WithContext(ctx).Create(budget).Error
}

// FindByIDAndUserID retrieves one budget visible to the user.
// The WHERE clause on both id AND user_id prevents horizontal privilege escalation.
func (r *postgresRepository) FindByIDAndUserID(ctx context.Context, id, userID uint) (*Budget, error) {
	var b Budget
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&b).Error
	return &b, err
}

// ListByUserID fetches all non-deleted budgets ordered by creation time descending.
// A separate COUNT query avoids an expensive ORDER BY for the count only.
func (r *postgresRepository) ListByUserID(ctx context.Context, userID uint) ([]Budget, int64, error) {
	var budgets []Budget
	var total int64

	if err := r.db.WithContext(ctx).
		Model(&Budget{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&budgets).Error; err != nil {
		return nil, 0, err
	}

	return budgets, total, nil
}

// Update applies only the keys present in the updates map (GORM map-based update).
// The WHERE clause includes user_id so a user cannot update another user's budget.
func (r *postgresRepository) Update(ctx context.Context, id, userID uint, updates map[string]interface{}) error {
	result := r.db.WithContext(ctx).
		Model(&Budget{}).
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

// Delete soft-deletes the budget by setting deleted_at.
// Returns gorm.ErrRecordNotFound when the row doesn't exist or is already deleted.
func (r *postgresRepository) Delete(ctx context.Context, id, userID uint) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&Budget{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetSpentAmount sums all non-deleted expense transactions in the given date
// window for a user. If categoryID is non-nil, only that category is counted.
//
// The query is against the `transactions` table using raw SQL so this package
// does not need to import the transactions package. COALESCE ensures a zero
// is returned instead of NULL when there are no matching rows.
func (r *postgresRepository) GetSpentAmount(
	ctx context.Context,
	userID uint,
	categoryID *uint,
	from, to time.Time,
) (float64, error) {
	query := r.db.WithContext(ctx).
		Table("transactions").
		Where(
			"user_id = ? AND type = 'expense' AND date >= ? AND date <= ? AND deleted_at IS NULL",
			userID, from.Format("2006-01-02"), to.Format("2006-01-02"),
		)

	// Only filter by category when one is specified; nil = all categories.
	if categoryID != nil {
		query = query.Where("category_id = ?", *categoryID)
	}

	var total float64
	if err := query.Select("COALESCE(SUM(amount), 0)").Row().Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}
