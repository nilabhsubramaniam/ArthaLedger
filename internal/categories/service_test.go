package categories_test

// Unit tests for the categories service layer.
//
// Key ownership rules exercised:
//   - System categories (IsSystem == true, UserID == nil) are read-only.
//   - A user may only update or delete categories she owns.
//   - GetByID returns ErrCategoryNotFound for wrong-owner categories (no 403 leakage).

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nilabh/arthaledger/internal/categories"
	"gorm.io/gorm"
)

// ── Mock repository ───────────────────────────────────────────────────────────

type mockCategoryRepo struct {
	createFn             func(ctx context.Context, cat *categories.Category) error
	findByIDFn           func(ctx context.Context, id uint) (*categories.Category, error)
	listByUserIDFn       func(ctx context.Context, userID uint) ([]categories.Category, error)
	listByUserAndTypeFn  func(ctx context.Context, userID uint, t categories.CategoryType) ([]categories.Category, error)
	updateFn             func(ctx context.Context, cat *categories.Category) error
	deleteFn             func(ctx context.Context, id uint) error
}

func (m *mockCategoryRepo) Create(ctx context.Context, cat *categories.Category) error {
	if m.createFn != nil {
		return m.createFn(ctx, cat)
	}
	cat.ID = 1
	cat.CreatedAt = time.Now()
	return nil
}

func (m *mockCategoryRepo) FindByID(ctx context.Context, id uint) (*categories.Category, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockCategoryRepo) ListByUserID(ctx context.Context, userID uint) ([]categories.Category, error) {
	if m.listByUserIDFn != nil {
		return m.listByUserIDFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockCategoryRepo) ListByUserIDAndType(ctx context.Context, userID uint, t categories.CategoryType) ([]categories.Category, error) {
	if m.listByUserAndTypeFn != nil {
		return m.listByUserAndTypeFn(ctx, userID, t)
	}
	return nil, nil
}

func (m *mockCategoryRepo) Update(ctx context.Context, cat *categories.Category) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, cat)
	}
	return nil
}

func (m *mockCategoryRepo) Delete(ctx context.Context, id uint) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func uid(x uint) *uint { return &x }

// systemCategory builds a read-only system category fixture.
func systemCategory(id uint) *categories.Category {
	return &categories.Category{
		ID:       id,
		UserID:   nil,
		Name:     "Groceries",
		Type:     categories.CategoryTypeExpense,
		IsSystem: true,
	}
}

// userCategory builds a user-owned category fixture.
func userCategory(id, ownerID uint) *categories.Category {
	return &categories.Category{
		ID:       id,
		UserID:   uid(ownerID),
		Name:     "Coffee",
		Type:     categories.CategoryTypeExpense,
		IsSystem: false,
	}
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestCategoryService_Create_Success(t *testing.T) {
	t.Parallel()
	svc := categories.NewService(&mockCategoryRepo{})

	resp, err := svc.Create(context.Background(), 1, categories.CreateCategoryRequest{
		Name: "Coffee",
		Type: categories.CategoryTypeExpense,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Name != "Coffee" {
		t.Errorf("name: got %q, want %q", resp.Name, "Coffee")
	}
	if resp.IsSystem {
		t.Error("user-created categories must not be marked as system")
	}
}

func TestCategoryService_Create_InvalidType(t *testing.T) {
	t.Parallel()
	svc := categories.NewService(&mockCategoryRepo{})

	_, err := svc.Create(context.Background(), 1, categories.CreateCategoryRequest{
		Name: "Bad",
		Type: "transfer", // not allowed
	})
	if !errors.Is(err, categories.ErrInvalidCategoryType) {
		t.Errorf("expected ErrInvalidCategoryType, got %v", err)
	}
}

// ── List tests ────────────────────────────────────────────────────────────────

func TestCategoryService_List_AllTypes(t *testing.T) {
	t.Parallel()
	repo := &mockCategoryRepo{
		listByUserIDFn: func(_ context.Context, _ uint) ([]categories.Category, error) {
			return []categories.Category{
				*systemCategory(1),
				*userCategory(2, 10),
			}, nil
		},
	}
	svc := categories.NewService(repo)

	list, err := svc.List(context.Background(), 10, "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if list.Total != 2 {
		t.Errorf("total: got %d, want 2", list.Total)
	}
}

func TestCategoryService_List_FilteredByType(t *testing.T) {
	t.Parallel()
	repo := &mockCategoryRepo{
		listByUserAndTypeFn: func(_ context.Context, _ uint, _ categories.CategoryType) ([]categories.Category, error) {
			return []categories.Category{*systemCategory(1)}, nil
		},
	}
	svc := categories.NewService(repo)

	list, err := svc.List(context.Background(), 10, "expense")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if list.Total != 1 {
		t.Errorf("total: got %d, want 1", list.Total)
	}
}

func TestCategoryService_List_InvalidFilterType(t *testing.T) {
	t.Parallel()
	svc := categories.NewService(&mockCategoryRepo{})

	_, err := svc.List(context.Background(), 1, "bogus")
	if !errors.Is(err, categories.ErrInvalidCategoryType) {
		t.Errorf("expected ErrInvalidCategoryType, got %v", err)
	}
}

// ── GetByID tests ─────────────────────────────────────────────────────────────

func TestCategoryService_GetByID_SystemCategoryVisible(t *testing.T) {
	t.Parallel()
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			return systemCategory(id), nil
		},
	}
	svc := categories.NewService(repo)

	// Any user can read a system category.
	resp, err := svc.GetByID(context.Background(), 42, 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !resp.IsSystem {
		t.Error("expected IsSystem == true for a system category")
	}
}

func TestCategoryService_GetByID_OwnedByUser(t *testing.T) {
	t.Parallel()
	const userID = uint(10)
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			return userCategory(id, userID), nil
		},
	}
	svc := categories.NewService(repo)

	resp, err := svc.GetByID(context.Background(), userID, 99)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if *resp.UserID != userID {
		t.Errorf("user_id: got %d, want %d", *resp.UserID, userID)
	}
}

func TestCategoryService_GetByID_WrongOwner(t *testing.T) {
	t.Parallel()
	// Category belongs to user 10, but user 99 is requesting.
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			return userCategory(id, 10), nil
		},
	}
	svc := categories.NewService(repo)

	_, err := svc.GetByID(context.Background(), 99, 1)
	if !errors.Is(err, categories.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound for wrong owner, got %v", err)
	}
}

func TestCategoryService_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	// Default mock returns gorm.ErrRecordNotFound → ErrCategoryNotFound
	svc := categories.NewService(&mockCategoryRepo{})

	_, err := svc.GetByID(context.Background(), 1, 999)
	if !errors.Is(err, categories.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got %v", err)
	}
}

// ── Update tests ──────────────────────────────────────────────────────────────

func TestCategoryService_Update_SystemCategory(t *testing.T) {
	t.Parallel()
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			return systemCategory(id), nil
		},
	}
	svc := categories.NewService(repo)

	_, err := svc.Update(context.Background(), 1, 1, categories.UpdateCategoryRequest{Name: "New"})
	if !errors.Is(err, categories.ErrSystemCategory) {
		t.Errorf("expected ErrSystemCategory, got %v", err)
	}
}

func TestCategoryService_Update_WrongOwner(t *testing.T) {
	t.Parallel()
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			return userCategory(id, 10), nil
		},
	}
	svc := categories.NewService(repo)

	_, err := svc.Update(context.Background(), 99, 1, categories.UpdateCategoryRequest{Name: "Hijack"})
	if !errors.Is(err, categories.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound for wrong owner, got %v", err)
	}
}

func TestCategoryService_Update_NoChanges(t *testing.T) {
	t.Parallel()
	// Category already has Name="Coffee", Icon="", Color="".
	// Sending the same empty values = nothing changed.
	const ownerID = uint(1)
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			cat := userCategory(id, ownerID)
			cat.Name = "Coffee"
			cat.Icon = ""
			cat.Color = ""
			return cat, nil
		},
	}
	svc := categories.NewService(repo)

	// Send an empty update request — no fields differ from existing values.
	_, err := svc.Update(context.Background(), ownerID, 1, categories.UpdateCategoryRequest{})
	if !errors.Is(err, categories.ErrNoUpdates) {
		t.Errorf("expected ErrNoUpdates, got %v", err)
	}
}

func TestCategoryService_Update_Success(t *testing.T) {
	t.Parallel()
	const ownerID = uint(1)
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			cat := userCategory(id, ownerID)
			cat.Name = "Old Name"
			return cat, nil
		},
	}
	svc := categories.NewService(repo)

	resp, err := svc.Update(context.Background(), ownerID, 1, categories.UpdateCategoryRequest{
		Name: "New Name",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Name != "New Name" {
		t.Errorf("name: got %q, want %q", resp.Name, "New Name")
	}
}

// ── Delete tests ──────────────────────────────────────────────────────────────

func TestCategoryService_Delete_SystemCategory(t *testing.T) {
	t.Parallel()
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			return systemCategory(id), nil
		},
	}
	svc := categories.NewService(repo)

	err := svc.Delete(context.Background(), 1, 1)
	if !errors.Is(err, categories.ErrSystemCategory) {
		t.Errorf("expected ErrSystemCategory, got %v", err)
	}
}

func TestCategoryService_Delete_WrongOwner(t *testing.T) {
	t.Parallel()
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			return userCategory(id, 10), nil // owned by user 10
		},
	}
	svc := categories.NewService(repo)

	err := svc.Delete(context.Background(), 99, 1) // user 99 tries to delete
	if !errors.Is(err, categories.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound for wrong owner, got %v", err)
	}
}

func TestCategoryService_Delete_Success(t *testing.T) {
	t.Parallel()
	const ownerID = uint(1)
	repo := &mockCategoryRepo{
		findByIDFn: func(_ context.Context, id uint) (*categories.Category, error) {
			return userCategory(id, ownerID), nil
		},
	}
	svc := categories.NewService(repo)

	err := svc.Delete(context.Background(), ownerID, 1)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCategoryService_Delete_NotFound(t *testing.T) {
	t.Parallel()
	svc := categories.NewService(&mockCategoryRepo{})

	err := svc.Delete(context.Background(), 1, 999)
	if !errors.Is(err, categories.ErrCategoryNotFound) {
		t.Errorf("expected ErrCategoryNotFound, got %v", err)
	}
}
