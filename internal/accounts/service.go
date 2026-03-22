package accounts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// ── Sentinel errors ────────────────────────────────────────────────────────────

var (
	// ErrAccountNotFound is returned when the account does not exist or belongs
	// to a different user. A single error prevents ID-enumeration attacks.
	ErrAccountNotFound = errors.New("account not found")

	// ErrInvalidAccountType is returned when the type field is not one of the
	// four allowed values (bank, cash, credit_card, investment).
	ErrInvalidAccountType = errors.New("invalid account type")

	// ErrHasTransactions is returned when the caller attempts to delete an account
	// that still has non-deleted transaction rows.
	ErrHasTransactions = errors.New("account has transactions — delete them first")

	// ErrNoUpdates is returned when a PUT request body contains no recognisable fields.
	ErrNoUpdates = errors.New("no updatable fields provided")
)

// ── Service interface ──────────────────────────────────────────────────────────

// Service is the business-logic contract for the accounts domain.
// Handlers depend on this interface, never on the concrete struct,
// making the service easily replaceable with a mock during testing.
type Service interface {
	Create(ctx context.Context, userID uint, req CreateAccountRequest) (*AccountResponse, error)
	List(ctx context.Context, userID uint) (*AccountListResponse, error)
	GetByID(ctx context.Context, id, userID uint) (*AccountResponse, error)
	Update(ctx context.Context, id, userID uint, req UpdateAccountRequest) (*AccountResponse, error)
	Delete(ctx context.Context, id, userID uint) error
	GetSummary(ctx context.Context, id, userID uint) (*AccountSummaryResponse, error)
}

// ── Concrete implementation ────────────────────────────────────────────────────

type service struct {
	repo Repository
}

// NewService constructs the accounts service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// ── Service method implementations ────────────────────────────────────────────

// Create validates the request and inserts a new account for the user.
//
// Business rules enforced here:
//   - AccountType must be one of the four valid values.
//   - InitialBalance seeds the balance column (useful for capturing existing accounts).
//   - Currency is stored exactly as provided (e.g. "INR", "USD").
func (s *service) Create(ctx context.Context, userID uint, req CreateAccountRequest) (*AccountResponse, error) {
	// Validate the account type before touching the DB.
	if !req.Type.IsValid() {
		return nil, ErrInvalidAccountType
	}

	account := &Account{
		UserID:   userID,
		Name:     req.Name,
		Type:     req.Type,
		Balance:  req.InitialBalance,
		Currency: req.Currency,
		IsActive: true,
	}

	if err := s.repo.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("creating account: %w", err)
	}

	slog.Info("Account created", "user_id", userID, "account_id", account.ID)
	resp := toAccountResponse(account)
	return &resp, nil
}

// List returns all accounts belonging to the user.
func (s *service) List(ctx context.Context, userID uint) (*AccountListResponse, error) {
	accounts, total, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing accounts: %w", err)
	}
	return &AccountListResponse{
		Accounts: toAccountResponseList(accounts),
		Total:    total,
	}, nil
}

// GetByID fetches a single account. Returns ErrAccountNotFound for any failure
// (non-existent or belonging to another user) to prevent information leakage.
func (s *service) GetByID(ctx context.Context, id, userID uint) (*AccountResponse, error) {
	account, err := s.repo.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("fetching account: %w", err)
	}
	resp := toAccountResponse(account)
	return &resp, nil
}

// Update applies partial changes to an account.
//
// Business rules enforced here:
//   - Balance and currency cannot be changed through this endpoint.
//   - If AccountType is provided, it must be a valid enum value.
//   - At least one field must be present in the request.
//   - Ownership is verified by the repository (user_id in WHERE clause).
func (s *service) Update(ctx context.Context, id, userID uint, req UpdateAccountRequest) (*AccountResponse, error) {
	// Build a map of only the fields that were actually sent in the request.
	// GORM's map-based update skips zero-value fields, so "false" for is_active must
	// use a pointer (*bool) in the request so we can distinguish "not sent" from "false".
	updates := map[string]interface{}{}

	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Type != "" {
		if !req.Type.IsValid() {
			return nil, ErrInvalidAccountType
		}
		updates["type"] = req.Type
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) == 0 {
		return nil, ErrNoUpdates
	}

	if err := s.repo.Update(ctx, id, userID, updates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("updating account: %w", err)
	}

	// Re-fetch to return the current state (including any DB defaults that may
	// have been applied and the new updated_at timestamp).
	return s.GetByID(ctx, id, userID)
}

// Delete soft-deletes an account.
//
// Business rule: accounts with existing (non-deleted) transactions cannot be
// deleted because the balance history would become orphaned.
// The caller must delete all transactions first, or we'd silently corrupt reports.
func (s *service) Delete(ctx context.Context, id, userID uint) error {
	// Ownership check first — ensure the account belongs to this user.
	if _, err := s.repo.FindByIDAndUserID(ctx, id, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("fetching account for delete: %w", err)
	}

	// Guard: refuse to delete if any non-deleted transactions reference this account.
	hasTx, err := s.repo.HasTransactions(ctx, id)
	if err != nil {
		return fmt.Errorf("checking transactions: %w", err)
	}
	if hasTx {
		return ErrHasTransactions
	}

	if err := s.repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("deleting account: %w", err)
	}

	slog.Info("Account soft-deleted", "account_id", id, "user_id", userID)
	return nil
}

// GetSummary returns an account plus aggregated transaction statistics.
func (s *service) GetSummary(ctx context.Context, id, userID uint) (*AccountSummaryResponse, error) {
	summary, err := s.repo.GetSummary(ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("fetching account summary: %w", err)
	}
	return summary, nil
}
