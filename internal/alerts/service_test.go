package alerts_test

// Unit tests for the alerts service layer.
//
// Coverage:
//   - List returns all alerts with total + unread counts.
//   - MarkRead returns ErrAlertNotFound for a missing or wrong-owner alert.
//   - MarkAllRead succeeds (no-op in mock).
//   - Delete returns ErrAlertNotFound for a missing or wrong-owner alert.
//   - CreateBudgetAlert:
//       • Skips inserting when an identical unread alert already exists (dedup).
//       • Inserts when no duplicate exists.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nilabh/arthaledger/internal/alerts"
	"gorm.io/gorm"
)

// ── Mock repository ───────────────────────────────────────────────────────────

type mockAlertRepo struct {
	createFn                  func(ctx context.Context, a *alerts.Alert) error
	listByUserIDFn            func(ctx context.Context, userID uint) ([]alerts.Alert, int64, int64, error)
	markReadFn                func(ctx context.Context, id, userID uint) error
	markAllReadFn             func(ctx context.Context, userID uint) error
	deleteFn                  func(ctx context.Context, id, userID uint) error
	existsUnreadForBudgetFn   func(ctx context.Context, userID, budgetID uint, alertType alerts.AlertType) (bool, error)

	createCallCount int // how many times Create was actually called
}

func (m *mockAlertRepo) Create(ctx context.Context, a *alerts.Alert) error {
	m.createCallCount++
	if m.createFn != nil {
		return m.createFn(ctx, a)
	}
	a.ID = 1
	a.CreatedAt = time.Now()
	return nil
}

func (m *mockAlertRepo) ListByUserID(ctx context.Context, userID uint) ([]alerts.Alert, int64, int64, error) {
	if m.listByUserIDFn != nil {
		return m.listByUserIDFn(ctx, userID)
	}
	return nil, 0, 0, nil
}

func (m *mockAlertRepo) MarkRead(ctx context.Context, id, userID uint) error {
	if m.markReadFn != nil {
		return m.markReadFn(ctx, id, userID)
	}
	return nil
}

func (m *mockAlertRepo) MarkAllRead(ctx context.Context, userID uint) error {
	if m.markAllReadFn != nil {
		return m.markAllReadFn(ctx, userID)
	}
	return nil
}

func (m *mockAlertRepo) Delete(ctx context.Context, id, userID uint) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id, userID)
	}
	return nil
}

func (m *mockAlertRepo) ExistsUnreadForBudget(ctx context.Context, userID, budgetID uint, alertType alerts.AlertType) (bool, error) {
	if m.existsUnreadForBudgetFn != nil {
		return m.existsUnreadForBudgetFn(ctx, userID, budgetID, alertType)
	}
	return false, nil
}

// ── List tests ────────────────────────────────────────────────────────────────

func TestAlertService_List_Success(t *testing.T) {
	t.Parallel()
	repo := &mockAlertRepo{
		listByUserIDFn: func(_ context.Context, _ uint) ([]alerts.Alert, int64, int64, error) {
			return []alerts.Alert{
				{ID: 1, Type: alerts.AlertBudgetWarning, Title: "Heads up", IsRead: false},
				{ID: 2, Type: alerts.AlertBudgetExceeded, Title: "Over limit", IsRead: true},
			}, 2, 1, nil
		},
	}
	svc := alerts.NewService(repo)

	list, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if list.Total != 2 {
		t.Errorf("total: got %d, want 2", list.Total)
	}
	if list.UnreadCount != 1 {
		t.Errorf("unread_count: got %d, want 1", list.UnreadCount)
	}
	if len(list.Alerts) != 2 {
		t.Errorf("len(alerts): got %d, want 2", len(list.Alerts))
	}
}

func TestAlertService_List_Empty(t *testing.T) {
	t.Parallel()
	svc := alerts.NewService(&mockAlertRepo{})

	list, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if list.Total != 0 {
		t.Errorf("total: got %d, want 0", list.Total)
	}
}

// ── MarkRead tests ────────────────────────────────────────────────────────────

func TestAlertService_MarkRead_Success(t *testing.T) {
	t.Parallel()
	svc := alerts.NewService(&mockAlertRepo{})

	if err := svc.MarkRead(context.Background(), 1, 1); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestAlertService_MarkRead_NotFound(t *testing.T) {
	t.Parallel()
	repo := &mockAlertRepo{
		markReadFn: func(_ context.Context, _, _ uint) error {
			return gorm.ErrRecordNotFound
		},
	}
	svc := alerts.NewService(repo)

	err := svc.MarkRead(context.Background(), 99, 1)
	if !errors.Is(err, alerts.ErrAlertNotFound) {
		t.Errorf("expected ErrAlertNotFound, got %v", err)
	}
}

// ── MarkAllRead tests ─────────────────────────────────────────────────────────

func TestAlertService_MarkAllRead_Success(t *testing.T) {
	t.Parallel()
	svc := alerts.NewService(&mockAlertRepo{})

	if err := svc.MarkAllRead(context.Background(), 1); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// ── Delete tests ──────────────────────────────────────────────────────────────

func TestAlertService_Delete_Success(t *testing.T) {
	t.Parallel()
	svc := alerts.NewService(&mockAlertRepo{})

	if err := svc.Delete(context.Background(), 1, 1); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestAlertService_Delete_NotFound(t *testing.T) {
	t.Parallel()
	repo := &mockAlertRepo{
		deleteFn: func(_ context.Context, _, _ uint) error {
			return gorm.ErrRecordNotFound
		},
	}
	svc := alerts.NewService(repo)

	err := svc.Delete(context.Background(), 99, 1)
	if !errors.Is(err, alerts.ErrAlertNotFound) {
		t.Errorf("expected ErrAlertNotFound, got %v", err)
	}
}

// ── CreateBudgetAlert tests ───────────────────────────────────────────────────

func TestAlertService_CreateBudgetAlert_Deduplication(t *testing.T) {
	t.Parallel()
	// ExistsUnreadForBudget returns true → Create must NOT be called.
	repo := &mockAlertRepo{
		existsUnreadForBudgetFn: func(_ context.Context, _, _ uint, _ alerts.AlertType) (bool, error) {
			return true, nil
		},
	}
	svc := alerts.NewService(repo)

	err := svc.CreateBudgetAlert(context.Background(), 1, 5, "budget_warning", "Heads up", "80% used")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.createCallCount != 0 {
		t.Errorf("Create should not be called when duplicate alert exists, got %d call(s)", repo.createCallCount)
	}
}

func TestAlertService_CreateBudgetAlert_InsertsWhenNoDuplicate(t *testing.T) {
	t.Parallel()
	// ExistsUnreadForBudget returns false → Create MUST be called once.
	repo := &mockAlertRepo{
		existsUnreadForBudgetFn: func(_ context.Context, _, _ uint, _ alerts.AlertType) (bool, error) {
			return false, nil
		},
	}
	svc := alerts.NewService(repo)

	err := svc.CreateBudgetAlert(context.Background(), 1, 5, "budget_exceeded", "Over limit!", "Spent 110%")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if repo.createCallCount != 1 {
		t.Errorf("Create should be called exactly once, got %d call(s)", repo.createCallCount)
	}
}

func TestAlertService_CreateBudgetAlert_ErrorFromExistsCheck(t *testing.T) {
	t.Parallel()
	// If the exists-check itself fails, the error must be propagated.
	dbErr := errors.New("connection reset by peer")
	repo := &mockAlertRepo{
		existsUnreadForBudgetFn: func(_ context.Context, _, _ uint, _ alerts.AlertType) (bool, error) {
			return false, dbErr
		},
	}
	svc := alerts.NewService(repo)

	err := svc.CreateBudgetAlert(context.Background(), 1, 5, "budget_warning", "t", "m")
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if repo.createCallCount != 0 {
		t.Error("Create must not be called when the exists-check fails")
	}
}
