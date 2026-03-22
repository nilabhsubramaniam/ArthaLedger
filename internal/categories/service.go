package categories

import (
	"context"
	"errors"
)

// Sentinel errors returned by the service layer and mapped to HTTP status codes
// by the handler.
var (
	// ErrCategoryNotFound is returned when the requested category does not exist
	// or belongs to a different user.
	ErrCategoryNotFound = errors.New("category not found")

	// ErrSystemCategory is returned when the caller attempts to modify or delete
	// a system-managed category (is_system == true).
	ErrSystemCategory = errors.New("system categories cannot be modified or deleted")

	// ErrInvalidCategoryType is returned when the 'type' field is not one of
	// the allowed values ("income" or "expense").
	ErrInvalidCategoryType = errors.New("category type must be 'income' or 'expense'")

	// ErrNoUpdates is returned when an update request contains no fields that
	// differ from the current values, preventing empty DB writes.
	ErrNoUpdates = errors.New("no fields to update")
)

// Service defines the business-logic operations for categories.
type Service interface {
	// Create validates and persists a new user-owned category.
	Create(ctx context.Context, userID uint, req CreateCategoryRequest) (*CategoryResponse, error)

	// List returns system categories + the user's own, optionally filtered by type.
	// Pass an empty string for categoryType to get all types.
	List(ctx context.Context, userID uint, categoryType string) (*CategoryListResponse, error)

	// GetByID returns a single category visible to the user (system or their own).
	GetByID(ctx context.Context, userID uint, categoryID uint) (*CategoryResponse, error)

	// Update patches the name, icon, or color of a user-owned category.
	// System categories and categories owned by other users are rejected.
	Update(ctx context.Context, userID uint, categoryID uint, req UpdateCategoryRequest) (*CategoryResponse, error)

	// Delete permanently removes a user-owned category.
	// System categories and categories owned by other users are rejected.
	Delete(ctx context.Context, userID uint, categoryID uint) error
}

// service is the concrete implementation of Service.
type service struct {
	repo Repository
}

// NewService creates a Service with the given Repository.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// Create validates the request and inserts a new category owned by userID.
func (s *service) Create(ctx context.Context, userID uint, req CreateCategoryRequest) (*CategoryResponse, error) {
	// Validate the type before touching the database
	if !req.Type.IsValid() {
		return nil, ErrInvalidCategoryType
	}

	cat := &Category{
		UserID:   &userID,
		Name:     req.Name,
		Type:     req.Type,
		Icon:     req.Icon,
		Color:    req.Color,
		IsSystem: false, // user-created categories are never system rows
	}
	if err := s.repo.Create(ctx, cat); err != nil {
		return nil, err
	}
	resp := toCategoryResponse(*cat)
	return &resp, nil
}

// List returns system + user categories. If categoryType is non-empty it is
// used as an additional filter.
func (s *service) List(ctx context.Context, userID uint, categoryType string) (*CategoryListResponse, error) {
	var (
		cats []Category
		err  error
	)

	if categoryType != "" {
		ct := CategoryType(categoryType)
		if !ct.IsValid() {
			return nil, ErrInvalidCategoryType
		}
		cats, err = s.repo.ListByUserIDAndType(ctx, userID, ct)
	} else {
		cats, err = s.repo.ListByUserID(ctx, userID)
	}
	if err != nil {
		return nil, err
	}

	return &CategoryListResponse{
		Categories: toCategoryResponseList(cats),
		Total:      len(cats),
	}, nil
}

// GetByID fetches a category that is either a system category or owned by userID.
// Returns ErrCategoryNotFound for any other case (including wrong owner).
func (s *service) GetByID(ctx context.Context, userID uint, categoryID uint) (*CategoryResponse, error) {
	cat, err := s.repo.FindByID(ctx, categoryID)
	if err != nil {
		return nil, ErrCategoryNotFound
	}

	// The category is accessible if it is a system row (UserID is nil) or
	// if the requesting user owns it.
	if cat.UserID != nil && *cat.UserID != userID {
		// Return 404 (not 403) to prevent ID enumeration
		return nil, ErrCategoryNotFound
	}

	resp := toCategoryResponse(*cat)
	return &resp, nil
}

// Update applies non-zero fields from the request to a user-owned category.
func (s *service) Update(ctx context.Context, userID uint, categoryID uint, req UpdateCategoryRequest) (*CategoryResponse, error) {
	cat, err := s.repo.FindByID(ctx, categoryID)
	if err != nil {
		return nil, ErrCategoryNotFound
	}

	// System categories are read-only
	if cat.IsSystem {
		return nil, ErrSystemCategory
	}

	// The category must belong to the requesting user
	if cat.UserID == nil || *cat.UserID != userID {
		return nil, ErrCategoryNotFound
	}

	// Apply only the fields the caller provided
	changed := false
	if req.Name != "" && req.Name != cat.Name {
		cat.Name = req.Name
		changed = true
	}
	if req.Icon != cat.Icon {
		cat.Icon = req.Icon
		changed = true
	}
	if req.Color != cat.Color {
		cat.Color = req.Color
		changed = true
	}
	if !changed {
		return nil, ErrNoUpdates
	}

	if err := s.repo.Update(ctx, cat); err != nil {
		return nil, err
	}
	resp := toCategoryResponse(*cat)
	return &resp, nil
}

// Delete permanently removes a user-owned category.
func (s *service) Delete(ctx context.Context, userID uint, categoryID uint) error {
	cat, err := s.repo.FindByID(ctx, categoryID)
	if err != nil {
		return ErrCategoryNotFound
	}

	// System categories cannot be deleted
	if cat.IsSystem {
		return ErrSystemCategory
	}

	// Must be the owner
	if cat.UserID == nil || *cat.UserID != userID {
		return ErrCategoryNotFound
	}

	return s.repo.Delete(ctx, categoryID)
}
