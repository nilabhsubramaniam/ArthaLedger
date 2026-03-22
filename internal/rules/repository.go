package rules

import (
	"context"

	"github.com/nilabh/arthaledger/pkg/categorizer"
	"gorm.io/gorm"
)

// Repository defines the data-access operations for categorization rules.
type Repository interface {
	// Create persists a new rule.
	Create(ctx context.Context, rule *Rule) error

	// FindByIDAndUserID fetches a single rule, scoped to the owner.
	FindByIDAndUserID(ctx context.Context, id, userID uint) (*Rule, error)

	// ListByUserID returns all rules for the given user, ordered by priority DESC then id ASC.
	ListByUserID(ctx context.Context, userID uint) ([]Rule, error)

	// ListAsCategorizer returns the rules for a user in the shape expected by
	// the categorizer package — avoids an extra allocation in the hot path.
	ListAsCategorizer(ctx context.Context, userID uint) ([]categorizer.Rule, error)

	// Delete permanently removes a rule.
	Delete(ctx context.Context, id uint) error
}

// postgresRepository is the PostgreSQL implementation of Repository.
type postgresRepository struct {
	db *gorm.DB
}

// NewRepository creates a new Repository backed by the given *gorm.DB.
func NewRepository(db *gorm.DB) Repository {
	return &postgresRepository{db: db}
}

// Create inserts a new rule row.
func (r *postgresRepository) Create(ctx context.Context, rule *Rule) error {
	return r.db.WithContext(ctx).Create(rule).Error
}

// FindByIDAndUserID fetches a rule that belongs to the given user.
func (r *postgresRepository) FindByIDAndUserID(ctx context.Context, id, userID uint) (*Rule, error) {
	var rule Rule
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&rule).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// ListByUserID returns all rules for the user ordered by priority DESC, id ASC.
func (r *postgresRepository) ListByUserID(ctx context.Context, userID uint) ([]Rule, error) {
	var rules []Rule
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("priority DESC, id ASC").
		Find(&rules).Error
	return rules, err
}

// ListAsCategorizer returns rules in the shape the categorizer package expects.
// This avoids scanning the full Rule struct when only the matching fields matter.
func (r *postgresRepository) ListAsCategorizer(ctx context.Context, userID uint) ([]categorizer.Rule, error) {
	// Fetch only the columns we need for the engine — leaner than loading full rows.
	type raw struct {
		ID         uint
		Keyword    string
		CategoryID uint
		Priority   int
	}
	var rows []raw
	err := r.db.WithContext(ctx).
		Model(&Rule{}).
		Select("id, keyword, category_id, priority").
		Where("user_id = ?", userID).
		Order("priority DESC, id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]categorizer.Rule, len(rows))
	for i, row := range rows {
		out[i] = categorizer.Rule{
			ID:         row.ID,
			Keyword:    row.Keyword,
			CategoryID: row.CategoryID,
			Priority:   row.Priority,
		}
	}
	return out, nil
}

// Delete permanently removes a rule by primary key.
func (r *postgresRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&Rule{}, id).Error
}
