package alerts

import (
	"context"

	"gorm.io/gorm"
)

// ── Interface ──────────────────────────────────────────────────────────────────

// Repository is the data-access contract for the alerts package.
type Repository interface {
	// Create inserts a new alert row.
	Create(ctx context.Context, alert *Alert) error

	// ListByUserID returns all alerts for the user, newest first.
	// Also returns total count and unread count for list-response metadata.
	ListByUserID(ctx context.Context, userID uint) ([]Alert, int64, int64, error)

	// MarkRead sets is_read = true for a single alert owned by the user.
	// Returns gorm.ErrRecordNotFound if the alert doesn't exist or belongs to
	// another user.
	MarkRead(ctx context.Context, id, userID uint) error

	// MarkAllRead sets is_read = true on every unread alert for the user.
	MarkAllRead(ctx context.Context, userID uint) error

	// Delete hard-deletes a single alert owned by the user.
	// Returns gorm.ErrRecordNotFound when absent or not owned by the user.
	Delete(ctx context.Context, id, userID uint) error

	// ExistsUnreadForBudget returns true when there is already at least one
	// unread alert of the given type linked to the given budget.
	// Used by the service to prevent duplicate alert spam on repeated list calls.
	ExistsUnreadForBudget(ctx context.Context, userID, budgetID uint, alertType AlertType) (bool, error)
}

// ── PostgreSQL implementation ──────────────────────────────────────────────────

type postgresRepository struct {
	db *gorm.DB
}

// NewRepository returns an alerts Repository backed by the given GORM handle.
func NewRepository(db *gorm.DB) Repository {
	return &postgresRepository{db: db}
}

// Create inserts the alert and lets PostgreSQL populate ID and CreatedAt.
func (r *postgresRepository) Create(ctx context.Context, alert *Alert) error {
	return r.db.WithContext(ctx).Create(alert).Error
}

// ListByUserID fetches all alerts for a user ordered newest first.
// Three values are returned: the rows, total count, and unread count.
func (r *postgresRepository) ListByUserID(ctx context.Context, userID uint) ([]Alert, int64, int64, error) {
	var alerts []Alert
	var total, unread int64

	// Total count
	if err := r.db.WithContext(ctx).
		Model(&Alert{}).
		Where("user_id = ?", userID).
		Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}

	// Unread count
	if err := r.db.WithContext(ctx).
		Model(&Alert{}).
		Where("user_id = ? AND is_read = false", userID).
		Count(&unread).Error; err != nil {
		return nil, 0, 0, err
	}

	// Fetch rows
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&alerts).Error; err != nil {
		return nil, 0, 0, err
	}

	return alerts, total, unread, nil
}

// MarkRead sets is_read = true for the alert matching both id AND user_id.
func (r *postgresRepository) MarkRead(ctx context.Context, id, userID uint) error {
	result := r.db.WithContext(ctx).
		Model(&Alert{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_read", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// MarkAllRead sets is_read = true on every unread alert for the user.
func (r *postgresRepository) MarkAllRead(ctx context.Context, userID uint) error {
	return r.db.WithContext(ctx).
		Model(&Alert{}).
		Where("user_id = ? AND is_read = false", userID).
		Update("is_read", true).Error
}

// Delete hard-deletes the alert row (no soft-delete for alerts).
func (r *postgresRepository) Delete(ctx context.Context, id, userID uint) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&Alert{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ExistsUnreadForBudget checks whether an unread alert of the given type already
// exists for the specified budget. This prevents duplicate alerts when the same
// budget is viewed multiple times while still over the threshold.
func (r *postgresRepository) ExistsUnreadForBudget(
	ctx context.Context,
	userID, budgetID uint,
	alertType AlertType,
) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&Alert{}).
		Where("user_id = ? AND budget_id = ? AND type = ? AND is_read = false", userID, budgetID, alertType).
		Count(&count).Error
	return count > 0, err
}
