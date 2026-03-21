package auth

import (
	"context"

	"gorm.io/gorm"
)

// ── Interface ──────────────────────────────────────────────────────────────────

// Repository is the data-access contract for the auth package.
// Declaring an interface (rather than calling GORM directly from the service)
// keeps business logic decoupled from the database driver and makes unit-testing
// straightforward — callers inject a mock that satisfies this interface.
type Repository interface {
	// Create inserts a new User row and populates the struct with the generated ID.
	Create(ctx context.Context, user *User) error

	// FindByEmail returns the user whose email matches, or gorm.ErrRecordNotFound.
	FindByEmail(ctx context.Context, email string) (*User, error)

	// FindByID returns the user with the given primary key, or gorm.ErrRecordNotFound.
	FindByID(ctx context.Context, id uint) (*User, error)
}

// ── PostgreSQL implementation ──────────────────────────────────────────────────

// postgresRepository is the GORM-backed implementation of Repository.
// Lowercase keeps it unexported; callers receive the Repository interface.
type postgresRepository struct {
	db *gorm.DB
}

// NewRepository returns a Repository backed by the supplied GORM database handle.
// This is the only constructor you need to call — you never touch postgresRepository directly.
func NewRepository(db *gorm.DB) Repository {
	return &postgresRepository{db: db}
}

// Create inserts a new user row.
// GORM populates user.ID, user.CreatedAt, and user.UpdatedAt after the INSERT.
// Email uniqueness is enforced at the database level (unique index on the column).
func (r *postgresRepository) Create(ctx context.Context, user *User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// FindByEmail returns the first active user whose email matches.
// Soft-deleted rows (deleted_at IS NOT NULL) are excluded by GORM automatically
// because the User model embeds gorm.DeletedAt.
func (r *postgresRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).
		Where("email = ?", email).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByID returns the first user matching the given primary-key value.
// Returns gorm.ErrRecordNotFound when no row exists.
func (r *postgresRepository) FindByID(ctx context.Context, id uint) (*User, error) {
	var user User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
