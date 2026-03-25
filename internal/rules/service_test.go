package rules_test

// Unit tests for the rules service layer.
//
// The rules service is thin — most complexity lives in the categorizer package
// which has its own test suite.  Here we verify:
//   - Create success and the duplicate-keyword conflict path.
//   - List returns the repository slice unchanged.
//   - Delete verifies ownership and maps not-found to ErrRuleNotFound.
//   - CategorizerRules delegates straight to the repository.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nilabh/arthaledger/internal/rules"
	"github.com/nilabh/arthaledger/pkg/categorizer"
	"gorm.io/gorm"
)

// ── Mock repository ───────────────────────────────────────────────────────────

type mockRuleRepo struct {
	createFn            func(ctx context.Context, r *rules.Rule) error
	findByIDAndUserIDFn func(ctx context.Context, id, userID uint) (*rules.Rule, error)
	listByUserIDFn      func(ctx context.Context, userID uint) ([]rules.Rule, error)
	listAsCategorizerFn func(ctx context.Context, userID uint) ([]categorizer.Rule, error)
	deleteFn            func(ctx context.Context, id uint) error
}

func (m *mockRuleRepo) Create(ctx context.Context, r *rules.Rule) error {
	if m.createFn != nil {
		return m.createFn(ctx, r)
	}
	r.ID = 1
	r.CreatedAt = time.Now()
	return nil
}

func (m *mockRuleRepo) FindByIDAndUserID(ctx context.Context, id, userID uint) (*rules.Rule, error) {
	if m.findByIDAndUserIDFn != nil {
		return m.findByIDAndUserIDFn(ctx, id, userID)
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockRuleRepo) ListByUserID(ctx context.Context, userID uint) ([]rules.Rule, error) {
	if m.listByUserIDFn != nil {
		return m.listByUserIDFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockRuleRepo) ListAsCategorizer(ctx context.Context, userID uint) ([]categorizer.Rule, error) {
	if m.listAsCategorizerFn != nil {
		return m.listAsCategorizerFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockRuleRepo) Delete(ctx context.Context, id uint) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestRuleService_Create_Success(t *testing.T) {
	t.Parallel()
	svc := rules.NewService(&mockRuleRepo{})

	resp, err := svc.Create(context.Background(), 1, rules.CreateRuleRequest{
		CategoryID: 10,
		Keyword:    "swiggy",
		Priority:   5,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Keyword != "swiggy" {
		t.Errorf("keyword: got %q, want %q", resp.Keyword, "swiggy")
	}
	if resp.CategoryID != 10 {
		t.Errorf("category_id: got %d, want 10", resp.CategoryID)
	}
}

func TestRuleService_Create_DuplicateKeyword(t *testing.T) {
	t.Parallel()
	repo := &mockRuleRepo{
		createFn: func(_ context.Context, _ *rules.Rule) error {
			// Simulate a PostgreSQL unique-violation (error code 23505).
			return errors.New("ERROR: duplicate key value violates unique constraint (SQLSTATE 23505)")
		},
	}
	svc := rules.NewService(repo)

	_, err := svc.Create(context.Background(), 1, rules.CreateRuleRequest{
		CategoryID: 10,
		Keyword:    "swiggy",
	})
	if !errors.Is(err, rules.ErrDuplicateKeyword) {
		t.Errorf("expected ErrDuplicateKeyword, got %v", err)
	}
}

// ── List tests ────────────────────────────────────────────────────────────────

func TestRuleService_List_ReturnsAll(t *testing.T) {
	t.Parallel()
	repo := &mockRuleRepo{
		listByUserIDFn: func(_ context.Context, _ uint) ([]rules.Rule, error) {
			return []rules.Rule{
				{ID: 1, UserID: 1, CategoryID: 10, Keyword: "swiggy", Priority: 5},
				{ID: 2, UserID: 1, CategoryID: 20, Keyword: "salary", Priority: 10},
			}, nil
		},
	}
	svc := rules.NewService(repo)

	list, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if list.Total != 2 {
		t.Errorf("total: got %d, want 2", list.Total)
	}
}

func TestRuleService_List_Empty(t *testing.T) {
	t.Parallel()
	svc := rules.NewService(&mockRuleRepo{})

	list, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if list.Total != 0 {
		t.Errorf("total: got %d, want 0", list.Total)
	}
}

// ── Delete tests ──────────────────────────────────────────────────────────────

func TestRuleService_Delete_Success(t *testing.T) {
	t.Parallel()
	repo := &mockRuleRepo{
		findByIDAndUserIDFn: func(_ context.Context, id, _ uint) (*rules.Rule, error) {
			return &rules.Rule{ID: id}, nil
		},
	}
	svc := rules.NewService(repo)

	if err := svc.Delete(context.Background(), 1, 1); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestRuleService_Delete_NotFound(t *testing.T) {
	t.Parallel()
	// Default mock returns gorm.ErrRecordNotFound from FindByIDAndUserID.
	svc := rules.NewService(&mockRuleRepo{})

	err := svc.Delete(context.Background(), 1, 99)
	if !errors.Is(err, rules.ErrRuleNotFound) {
		t.Errorf("expected ErrRuleNotFound, got %v", err)
	}
}

// ── CategorizerRules tests ────────────────────────────────────────────────────

func TestRuleService_CategorizerRules_ReturnsSlice(t *testing.T) {
	t.Parallel()
	expected := []categorizer.Rule{
		{ID: 1, Keyword: "swiggy", CategoryID: 10, Priority: 5},
		{ID: 2, Keyword: "salary", CategoryID: 20, Priority: 10},
	}
	repo := &mockRuleRepo{
		listAsCategorizerFn: func(_ context.Context, _ uint) ([]categorizer.Rule, error) {
			return expected, nil
		},
	}
	svc := rules.NewService(repo)

	got, err := svc.CategorizerRules(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len: got %d, want 2", len(got))
	}
	for i, r := range got {
		if r.Keyword != expected[i].Keyword {
			t.Errorf("[%d] keyword: got %q, want %q", i, r.Keyword, expected[i].Keyword)
		}
	}
}

func TestRuleService_CategorizerRules_Empty(t *testing.T) {
	t.Parallel()
	svc := rules.NewService(&mockRuleRepo{})

	got, err := svc.CategorizerRules(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d rules", len(got))
	}
}
