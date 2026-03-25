package budgets_test

// Unit tests for the budgets service layer.
//
// buildResponse calls GetSpentAmount and (optionally) fires alerts.  The tests
// cover:
//   - Input validation errors (invalid period, bad date, end before start)
//   - GetByID not found
//   - Update with no fields → ErrNoUpdates
//   - Delete not found    → ErrBudgetNotFound
//   - buildResponse fires NO alert when spent < 80 %
//   - buildResponse fires a budget_warning when spent ≥ 80 %
//   - buildResponse fires a budget_exceeded when spent ≥ 100 % (limit)
//   - Alert deduplication is NOT the budget service's responsibility
//     (that lives in alerts.Service); these tests only verify whether
//     alertCreator.CreateBudgetAlert is called or not.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nilabh/arthaledger/internal/budgets"
	"gorm.io/gorm"
)

// ── Mock repository ───────────────────────────────────────────────────────────

type mockBudgetRepo struct {
	createFn            func(ctx context.Context, b *budgets.Budget) error
	findByIDAndUserIDFn func(ctx context.Context, id, userID uint) (*budgets.Budget, error)
	listByUserIDFn      func(ctx context.Context, userID uint) ([]budgets.Budget, int64, error)
	updateFn            func(ctx context.Context, id, userID uint, updates map[string]interface{}) error
	deleteFn            func(ctx context.Context, id, userID uint) error
	getSpentFn          func(ctx context.Context, userID uint, categoryID *uint, from, to time.Time) (float64, error)
}

func (m *mockBudgetRepo) Create(ctx context.Context, b *budgets.Budget) error {
	if m.createFn != nil {
		return m.createFn(ctx, b)
	}
	b.ID = 1
	return nil
}
func (m *mockBudgetRepo) FindByIDAndUserID(ctx context.Context, id, userID uint) (*budgets.Budget, error) {
	if m.findByIDAndUserIDFn != nil {
		return m.findByIDAndUserIDFn(ctx, id, userID)
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *mockBudgetRepo) ListByUserID(ctx context.Context, userID uint) ([]budgets.Budget, int64, error) {
	if m.listByUserIDFn != nil {
		return m.listByUserIDFn(ctx, userID)
	}
	return nil, 0, nil
}
func (m *mockBudgetRepo) Update(ctx context.Context, id, userID uint, updates map[string]interface{}) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, userID, updates)
	}
	return nil
}
func (m *mockBudgetRepo) Delete(ctx context.Context, id, userID uint) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id, userID)
	}
	return nil
}
func (m *mockBudgetRepo) GetSpentAmount(ctx context.Context, userID uint, categoryID *uint, from, to time.Time) (float64, error) {
	if m.getSpentFn != nil {
		return m.getSpentFn(ctx, userID, categoryID, from, to)
	}
	return 0, nil
}

// ── Mock alert creator ────────────────────────────────────────────────────────

type mockAlertCreator struct {
	calls []alertCall
}

type alertCall struct {
	budgetID  uint
	alertType string
}

func (m *mockAlertCreator) CreateBudgetAlert(
	_ context.Context, _ uint, budgetID uint, alertType, _, _ string,
) error {
	m.calls = append(m.calls, alertCall{budgetID: budgetID, alertType: alertType})
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// newBasicBudget returns a minimal budget fixture with the given spent mock.
func newBasicBudget(id uint, amount float64, spent float64) (*mockBudgetRepo, *mockAlertCreator) {
	repo := &mockBudgetRepo{
		findByIDAndUserIDFn: func(_ context.Context, budgetID, _ uint) (*budgets.Budget, error) {
			return &budgets.Budget{
				ID:        budgetID,
				UserID:    1,
				Name:      "Test Budget",
				Amount:    amount,
				Period:    budgets.PeriodMonthly,
				StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				IsActive:  true,
			}, nil
		},
		getSpentFn: func(_ context.Context, _ uint, _ *uint, _, _ time.Time) (float64, error) {
			return spent, nil
		},
	}
	creator := &mockAlertCreator{}
	return repo, creator
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestBudgetService_Create_Success(t *testing.T) {
	t.Parallel()
	repo := &mockBudgetRepo{
		getSpentFn: func(_ context.Context, _ uint, _ *uint, _, _ time.Time) (float64, error) {
			return 0, nil
		},
	}
	svc := budgets.NewService(repo, nil)

	resp, err := svc.Create(context.Background(), 1, budgets.CreateBudgetRequest{
		Name:      "Groceries",
		Amount:    5000,
		Period:    budgets.PeriodMonthly,
		StartDate: "2024-01-01",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Name != "Groceries" {
		t.Errorf("name: got %q, want %q", resp.Name, "Groceries")
	}
}

func TestBudgetService_Create_InvalidPeriod(t *testing.T) {
	t.Parallel()
	svc := budgets.NewService(&mockBudgetRepo{}, nil)

	_, err := svc.Create(context.Background(), 1, budgets.CreateBudgetRequest{
		Name:      "Bad Budget",
		Amount:    100,
		Period:    "bi-weekly", // invalid
		StartDate: "2024-01-01",
	})
	if !errors.Is(err, budgets.ErrInvalidPeriod) {
		t.Errorf("expected ErrInvalidPeriod, got %v", err)
	}
}

func TestBudgetService_Create_InvalidDate(t *testing.T) {
	t.Parallel()
	svc := budgets.NewService(&mockBudgetRepo{}, nil)

	_, err := svc.Create(context.Background(), 1, budgets.CreateBudgetRequest{
		Name:      "Budget",
		Amount:    100,
		Period:    budgets.PeriodMonthly,
		StartDate: "not-a-date",
	})
	if !errors.Is(err, budgets.ErrInvalidDate) {
		t.Errorf("expected ErrInvalidDate, got %v", err)
	}
}

func TestBudgetService_Create_EndBeforeStart(t *testing.T) {
	t.Parallel()
	svc := budgets.NewService(&mockBudgetRepo{}, nil)

	_, err := svc.Create(context.Background(), 1, budgets.CreateBudgetRequest{
		Name:      "Budget",
		Amount:    100,
		Period:    budgets.PeriodMonthly,
		StartDate: "2024-06-01",
		EndDate:   "2024-05-01", // before start
	})
	if !errors.Is(err, budgets.ErrEndBeforeStart) {
		t.Errorf("expected ErrEndBeforeStart, got %v", err)
	}
}

// ── GetByID tests ─────────────────────────────────────────────────────────────

func TestBudgetService_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	svc := budgets.NewService(&mockBudgetRepo{}, nil)

	_, err := svc.GetByID(context.Background(), 99, 1)
	if !errors.Is(err, budgets.ErrBudgetNotFound) {
		t.Errorf("expected ErrBudgetNotFound, got %v", err)
	}
}

func TestBudgetService_GetByID_Success(t *testing.T) {
	t.Parallel()
	repo, _ := newBasicBudget(1, 5000, 1000)
	svc := budgets.NewService(repo, nil)

	resp, err := svc.GetByID(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Spent != 1000 {
		t.Errorf("spent: got %v, want 1000", resp.Spent)
	}
}

// ── Alert-threshold tests ─────────────────────────────────────────────────────

func TestBudgetService_GetByID_NoAlertBelow80(t *testing.T) {
	t.Parallel()
	repo, creator := newBasicBudget(1, 5000, 3999) // 79.98 % used
	svc := budgets.NewService(repo, creator)

	_, err := svc.GetByID(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creator.calls) > 0 {
		t.Errorf("expected no alert below 80%%, got %d alert(s)", len(creator.calls))
	}
}

func TestBudgetService_GetByID_WarningAt80Percent(t *testing.T) {
	t.Parallel()
	repo, creator := newBasicBudget(1, 5000, 4000) // exactly 80 %
	svc := budgets.NewService(repo, creator)

	_, err := svc.GetByID(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creator.calls) == 0 {
		t.Fatal("expected at least one alert at 80% usage")
	}
	if creator.calls[0].alertType != "budget_warning" {
		t.Errorf("alertType: got %q, want %q", creator.calls[0].alertType, "budget_warning")
	}
}

func TestBudgetService_GetByID_ExceededAt100Percent(t *testing.T) {
	t.Parallel()
	repo, creator := newBasicBudget(1, 5000, 5500) // 110 % — over limit
	svc := budgets.NewService(repo, creator)

	_, err := svc.GetByID(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creator.calls) == 0 {
		t.Fatal("expected at least one alert when budget is exceeded")
	}
	if creator.calls[0].alertType != "budget_exceeded" {
		t.Errorf("alertType: got %q, want %q", creator.calls[0].alertType, "budget_exceeded")
	}
}

func TestBudgetService_GetByID_NoAlertWhenInactive(t *testing.T) {
	t.Parallel()
	creator := &mockAlertCreator{}
	repo := &mockBudgetRepo{
		findByIDAndUserIDFn: func(_ context.Context, id, _ uint) (*budgets.Budget, error) {
			return &budgets.Budget{
				ID:        id,
				Amount:    1000,
				Period:    budgets.PeriodMonthly,
				StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				IsActive:  false, // inactive — no alerts should fire
			}, nil
		},
		getSpentFn: func(_ context.Context, _ uint, _ *uint, _, _ time.Time) (float64, error) {
			return 1000, nil // 100 % spent
		},
	}
	svc := budgets.NewService(repo, creator)

	_, err := svc.GetByID(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(creator.calls) > 0 {
		t.Errorf("expected no alerts for inactive budget, got %d", len(creator.calls))
	}
}

// ── Update tests ──────────────────────────────────────────────────────────────

func TestBudgetService_Update_NoFields(t *testing.T) {
	t.Parallel()
	svc := budgets.NewService(&mockBudgetRepo{}, nil)

	_, err := svc.Update(context.Background(), 1, 1, budgets.UpdateBudgetRequest{})
	if !errors.Is(err, budgets.ErrNoUpdates) {
		t.Errorf("expected ErrNoUpdates, got %v", err)
	}
}

func TestBudgetService_Update_InvalidPeriod(t *testing.T) {
	t.Parallel()
	svc := budgets.NewService(&mockBudgetRepo{}, nil)

	_, err := svc.Update(context.Background(), 1, 1, budgets.UpdateBudgetRequest{
		Period: "bi-weekly",
	})
	if !errors.Is(err, budgets.ErrInvalidPeriod) {
		t.Errorf("expected ErrInvalidPeriod, got %v", err)
	}
}

func TestBudgetService_Update_InvalidEndDate(t *testing.T) {
	t.Parallel()
	svc := budgets.NewService(&mockBudgetRepo{}, nil)

	_, err := svc.Update(context.Background(), 1, 1, budgets.UpdateBudgetRequest{
		EndDate: "not-a-date",
	})
	if !errors.Is(err, budgets.ErrInvalidDate) {
		t.Errorf("expected ErrInvalidDate, got %v", err)
	}
}

func TestBudgetService_Update_NotFound(t *testing.T) {
	t.Parallel()
	repo := &mockBudgetRepo{
		updateFn: func(_ context.Context, _, _ uint, _ map[string]interface{}) error {
			return gorm.ErrRecordNotFound
		},
	}
	svc := budgets.NewService(repo, nil)

	_, err := svc.Update(context.Background(), 99, 1, budgets.UpdateBudgetRequest{Name: "x"})
	if !errors.Is(err, budgets.ErrBudgetNotFound) {
		t.Errorf("expected ErrBudgetNotFound, got %v", err)
	}
}

// ── Delete tests ──────────────────────────────────────────────────────────────

func TestBudgetService_Delete_NotFound(t *testing.T) {
	t.Parallel()
	repo := &mockBudgetRepo{
		deleteFn: func(_ context.Context, _, _ uint) error {
			return gorm.ErrRecordNotFound
		},
	}
	svc := budgets.NewService(repo, nil)

	err := svc.Delete(context.Background(), 99, 1)
	if !errors.Is(err, budgets.ErrBudgetNotFound) {
		t.Errorf("expected ErrBudgetNotFound, got %v", err)
	}
}

func TestBudgetService_Delete_Success(t *testing.T) {
	t.Parallel()
	svc := budgets.NewService(&mockBudgetRepo{}, nil)

	if err := svc.Delete(context.Background(), 1, 1); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// ── List tests ────────────────────────────────────────────────────────────────

func TestBudgetService_List_Success(t *testing.T) {
	t.Parallel()
	repo := &mockBudgetRepo{
		listByUserIDFn: func(_ context.Context, _ uint) ([]budgets.Budget, int64, error) {
			return []budgets.Budget{
				{
					ID: 1, Name: "Groceries", Amount: 5000, Period: budgets.PeriodMonthly,
					StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), IsActive: true,
				},
			}, 1, nil
		},
		getSpentFn: func(_ context.Context, _ uint, _ *uint, _, _ time.Time) (float64, error) {
			return 0, nil
		},
	}
	svc := budgets.NewService(repo, nil)

	list, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if list.Total != 1 {
		t.Errorf("total: got %d, want 1", list.Total)
	}
}
