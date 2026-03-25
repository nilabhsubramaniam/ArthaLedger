package accounts_test

// Unit tests for the accounts service layer.
//
// The mock repository implements every method of accounts.Repository.
// Tests for DB-transaction paths (UpdateBalance, DB()) are omitted because
// they require a real *gorm.DB; all other branches are covered here.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nilabh/arthaledger/internal/accounts"
	"gorm.io/gorm"
)

// ── Mock repository ───────────────────────────────────────────────────────────

type mockAccountRepo struct {
	createFn            func(ctx context.Context, a *accounts.Account) error
	findByIDAndUserIDFn func(ctx context.Context, id, userID uint) (*accounts.Account, error)
	listByUserIDFn      func(ctx context.Context, userID uint) ([]accounts.Account, int64, error)
	updateFn            func(ctx context.Context, id, userID uint, updates map[string]interface{}) error
	deleteFn            func(ctx context.Context, id, userID uint) error
	updateBalanceFn     func(ctx context.Context, tx *gorm.DB, accountID uint, delta float64) error
	hasTransactionsFn   func(ctx context.Context, accountID uint) (bool, error)
	getSummaryFn        func(ctx context.Context, id, userID uint) (*accounts.AccountSummaryResponse, error)
}

func (m *mockAccountRepo) Create(ctx context.Context, a *accounts.Account) error {
	if m.createFn != nil {
		return m.createFn(ctx, a)
	}
	a.ID = 1
	a.CreatedAt = time.Now()
	a.UpdatedAt = time.Now()
	return nil
}

func (m *mockAccountRepo) FindByIDAndUserID(ctx context.Context, id, userID uint) (*accounts.Account, error) {
	if m.findByIDAndUserIDFn != nil {
		return m.findByIDAndUserIDFn(ctx, id, userID)
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockAccountRepo) ListByUserID(ctx context.Context, userID uint) ([]accounts.Account, int64, error) {
	if m.listByUserIDFn != nil {
		return m.listByUserIDFn(ctx, userID)
	}
	return nil, 0, nil
}

func (m *mockAccountRepo) Update(ctx context.Context, id, userID uint, updates map[string]interface{}) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, userID, updates)
	}
	return nil
}

func (m *mockAccountRepo) Delete(ctx context.Context, id, userID uint) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id, userID)
	}
	return nil
}

func (m *mockAccountRepo) UpdateBalance(ctx context.Context, tx *gorm.DB, accountID uint, delta float64) error {
	if m.updateBalanceFn != nil {
		return m.updateBalanceFn(ctx, tx, accountID, delta)
	}
	return nil
}

func (m *mockAccountRepo) HasTransactions(ctx context.Context, accountID uint) (bool, error) {
	if m.hasTransactionsFn != nil {
		return m.hasTransactionsFn(ctx, accountID)
	}
	return false, nil
}

func (m *mockAccountRepo) GetSummary(ctx context.Context, id, userID uint) (*accounts.AccountSummaryResponse, error) {
	if m.getSummaryFn != nil {
		return m.getSummaryFn(ctx, id, userID)
	}
	return &accounts.AccountSummaryResponse{}, nil
}

func (m *mockAccountRepo) DB() *gorm.DB {
	// Returns nil — callers that open a DB transaction will panic.
	// Tests that reach this path must not be run in this test file.
	return nil
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestAccountService_Create_Success(t *testing.T) {
	t.Parallel()
	svc := accounts.NewService(&mockAccountRepo{})

	resp, err := svc.Create(context.Background(), 1, accounts.CreateAccountRequest{
		Name:           "HDFC Savings",
		Type:           accounts.AccountTypeBank,
		Currency:       "INR",
		InitialBalance: 5000,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Name != "HDFC Savings" {
		t.Errorf("name: got %q, want %q", resp.Name, "HDFC Savings")
	}
	if resp.Balance != 5000 {
		t.Errorf("balance: got %v, want 5000", resp.Balance)
	}
}

func TestAccountService_Create_InvalidType(t *testing.T) {
	t.Parallel()
	svc := accounts.NewService(&mockAccountRepo{})

	_, err := svc.Create(context.Background(), 1, accounts.CreateAccountRequest{
		Name:     "Bad Account",
		Type:     "piggy_bank", // invalid
		Currency: "INR",
	})
	if !errors.Is(err, accounts.ErrInvalidAccountType) {
		t.Errorf("expected ErrInvalidAccountType, got %v", err)
	}
}

// ── List tests ────────────────────────────────────────────────────────────────

func TestAccountService_List_ReturnsAll(t *testing.T) {
	t.Parallel()
	repo := &mockAccountRepo{
		listByUserIDFn: func(_ context.Context, _ uint) ([]accounts.Account, int64, error) {
			return []accounts.Account{
				{ID: 1, Name: "Wallet", Type: accounts.AccountTypeCash, Currency: "INR"},
				{ID: 2, Name: "HDFC",   Type: accounts.AccountTypeBank, Currency: "INR"},
			}, 2, nil
		},
	}
	svc := accounts.NewService(repo)

	list, err := svc.List(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if list.Total != 2 {
		t.Errorf("total: got %d, want 2", list.Total)
	}
	if len(list.Accounts) != 2 {
		t.Errorf("len(accounts): got %d, want 2", len(list.Accounts))
	}
}

// ── GetByID tests ─────────────────────────────────────────────────────────────

func TestAccountService_GetByID_Success(t *testing.T) {
	t.Parallel()
	repo := &mockAccountRepo{
		findByIDAndUserIDFn: func(_ context.Context, id, _ uint) (*accounts.Account, error) {
			return &accounts.Account{ID: id, Name: "HDFC", Type: accounts.AccountTypeBank}, nil
		},
	}
	svc := accounts.NewService(repo)

	resp, err := svc.GetByID(context.Background(), 5, 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.ID != 5 {
		t.Errorf("id: got %d, want 5", resp.ID)
	}
}

func TestAccountService_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	// Default mockAccountRepo.FindByIDAndUserID returns gorm.ErrRecordNotFound.
	svc := accounts.NewService(&mockAccountRepo{})

	_, err := svc.GetByID(context.Background(), 99, 1)
	if !errors.Is(err, accounts.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got %v", err)
	}
}

// ── Update tests ──────────────────────────────────────────────────────────────

func TestAccountService_Update_NoFields(t *testing.T) {
	t.Parallel()
	// An empty request body → nothing to update.
	svc := accounts.NewService(&mockAccountRepo{})

	_, err := svc.Update(context.Background(), 1, 1, accounts.UpdateAccountRequest{})
	if !errors.Is(err, accounts.ErrNoUpdates) {
		t.Errorf("expected ErrNoUpdates, got %v", err)
	}
}

func TestAccountService_Update_InvalidType(t *testing.T) {
	t.Parallel()
	svc := accounts.NewService(&mockAccountRepo{})

	_, err := svc.Update(context.Background(), 1, 1, accounts.UpdateAccountRequest{
		Type: "invalid",
	})
	if !errors.Is(err, accounts.ErrInvalidAccountType) {
		t.Errorf("expected ErrInvalidAccountType, got %v", err)
	}
}

func TestAccountService_Update_Success(t *testing.T) {
	t.Parallel()
	updatedName := "Renamed Account"
	repo := &mockAccountRepo{
		updateFn: func(_ context.Context, _, _ uint, _ map[string]interface{}) error {
			return nil
		},
		findByIDAndUserIDFn: func(_ context.Context, id, _ uint) (*accounts.Account, error) {
			// Return the updated account on re-fetch.
			return &accounts.Account{
				ID:   id,
				Name: updatedName,
				Type: accounts.AccountTypeBank,
			}, nil
		},
	}
	svc := accounts.NewService(repo)

	resp, err := svc.Update(context.Background(), 1, 1, accounts.UpdateAccountRequest{
		Name: updatedName,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if resp.Name != updatedName {
		t.Errorf("name: got %q, want %q", resp.Name, updatedName)
	}
}

func TestAccountService_Update_NotFound(t *testing.T) {
	t.Parallel()
	repo := &mockAccountRepo{
		updateFn: func(_ context.Context, _, _ uint, _ map[string]interface{}) error {
			return gorm.ErrRecordNotFound
		},
	}
	svc := accounts.NewService(repo)

	_, err := svc.Update(context.Background(), 99, 1, accounts.UpdateAccountRequest{Name: "x"})
	if !errors.Is(err, accounts.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got %v", err)
	}
}

// ── Delete tests ──────────────────────────────────────────────────────────────

func TestAccountService_Delete_Success(t *testing.T) {
	t.Parallel()
	repo := &mockAccountRepo{
		findByIDAndUserIDFn: func(_ context.Context, id, _ uint) (*accounts.Account, error) {
			return &accounts.Account{ID: id}, nil
		},
		hasTransactionsFn: func(_ context.Context, _ uint) (bool, error) {
			return false, nil
		},
	}
	svc := accounts.NewService(repo)

	if err := svc.Delete(context.Background(), 1, 1); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestAccountService_Delete_NotFound(t *testing.T) {
	t.Parallel()
	// FindByIDAndUserID returns not-found by default.
	svc := accounts.NewService(&mockAccountRepo{})

	err := svc.Delete(context.Background(), 99, 1)
	if !errors.Is(err, accounts.ErrAccountNotFound) {
		t.Errorf("expected ErrAccountNotFound, got %v", err)
	}
}

func TestAccountService_Delete_HasTransactions(t *testing.T) {
	t.Parallel()
	repo := &mockAccountRepo{
		findByIDAndUserIDFn: func(_ context.Context, id, _ uint) (*accounts.Account, error) {
			return &accounts.Account{ID: id}, nil
		},
		hasTransactionsFn: func(_ context.Context, _ uint) (bool, error) {
			return true, nil // account still has transactions
		},
	}
	svc := accounts.NewService(repo)

	err := svc.Delete(context.Background(), 1, 1)
	if !errors.Is(err, accounts.ErrHasTransactions) {
		t.Errorf("expected ErrHasTransactions, got %v", err)
	}
}

// ── GetSummary tests ──────────────────────────────────────────────────────────

func TestAccountService_GetSummary_Success(t *testing.T) {
	t.Parallel()
	repo := &mockAccountRepo{
		getSummaryFn: func(_ context.Context, id, _ uint) (*accounts.AccountSummaryResponse, error) {
			return &accounts.AccountSummaryResponse{
				TransactionCount: 3,
			}, nil
		},
	}
	svc := accounts.NewService(repo)

	summary, err := svc.GetSummary(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if summary.TransactionCount != 3 {
		t.Errorf("transaction_count: got %d, want 3", summary.TransactionCount)
	}
}

func TestAccountService_GetSummary_NotFound(t *testing.T) {
	t.Parallel()
	repo := &mockAccountRepo{
		getSummaryFn: func(_ context.Context, _, _ uint) (*accounts.AccountSummaryResponse, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := accounts.NewService(repo)

	_, err := svc.GetSummary(context.Background(), 99, 1)
	if err == nil {
		t.Error("expected an error, got nil")
	}
}
