package categories

import (
	"context"

	"gorm.io/gorm"
)

// Repository defines the data-access operations for categories.
// All list queries return both system categories (user_id IS NULL) and the
// requesting user's own categories so the client has one combined list.
type Repository interface {
	// Create persists a new user category.
	Create(ctx context.Context, cat *Category) error

	// FindByID returns a category by primary key regardless of ownership.
	// Used internally to validate before update/delete.
	FindByID(ctx context.Context, id uint) (*Category, error)

	// ListByUserID returns all system categories UNION all categories
	// created by the given user, ordered by type then name.
	ListByUserID(ctx context.Context, userID uint) ([]Category, error)

	// ListByUserIDAndType filters the combined list to a single CategoryType.
	ListByUserIDAndType(ctx context.Context, userID uint, t CategoryType) ([]Category, error)

	// Update saves changes to a user category (name / icon / color only).
	Update(ctx context.Context, cat *Category) error

	// Delete hard-deletes a user category. System categories must be rejected
	// at the service layer before this is called.
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

// Create inserts a new category row.
func (r *postgresRepository) Create(ctx context.Context, cat *Category) error {
	return r.db.WithContext(ctx).Create(cat).Error
}

// FindByID fetches a single category by its primary key.
func (r *postgresRepository) FindByID(ctx context.Context, id uint) (*Category, error) {
	var cat Category
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&cat).Error
	if err != nil {
		return nil, err
	}
	return &cat, nil
}

// ListByUserID returns system categories AND the user's own categories combined.
//
// SQL equivalent:
//
//	SELECT * FROM categories
//	WHERE user_id IS NULL OR user_id = ?
//	ORDER BY type, name
func (r *postgresRepository) ListByUserID(ctx context.Context, userID uint) ([]Category, error) {
	var cats []Category
	err := r.db.WithContext(ctx).
		Where("user_id IS NULL OR user_id = ?", userID).
		Order("type ASC, name ASC").
		Find(&cats).Error
	return cats, err
}

// ListByUserIDAndType is like ListByUserID but filtered to one CategoryType.
func (r *postgresRepository) ListByUserIDAndType(ctx context.Context, userID uint, t CategoryType) ([]Category, error) {
	var cats []Category
	err := r.db.WithContext(ctx).
		Where("(user_id IS NULL OR user_id = ?) AND type = ?", userID, t).
		Order("name ASC").
		Find(&cats).Error
	return cats, err
}

// Update saves name / icon / color changes to an existing category.
func (r *postgresRepository) Update(ctx context.Context, cat *Category) error {
	return r.db.WithContext(ctx).Save(cat).Error
}

// Delete permanently removes a category row by primary key.
func (r *postgresRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).
		Delete(&Category{}, id).Error
}
