package transactions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nilabh/arthaledger/internal/accounts"
	"gorm.io/gorm"
)

// ── Sentinel errors ────────────────────────────────────────────────────────────

var (
	// ErrTransactionNotFound is returned when the transaction does not exist or
	// belongs to a different user (same error prevents ID enumeration).
	ErrTransactionNotFound = errors.New("transaction not found")

	// ErrInvalidTransactionType is returned when the type field is not one of:
	// income, expense, transfer.
	ErrInvalidTransactionType = errors.New("invalid transaction type")

	// ErrTransferRequiresToAccount is returned when type=transfer but
	// to_account_id is missing from the request.
	ErrTransferRequiresToAccount = errors.New("transfer requires to_account_id")

	// ErrSameAccountTransfer is returned when source and destination account are
	// the same — a self-transfer has no financial meaning.
	ErrSameAccountTransfer = errors.New("source and destination accounts must be different")

	// ErrInvalidDate is returned when the date string cannot be parsed as YYYY-MM-DD.
	ErrInvalidDate = errors.New("invalid date format, expected YYYY-MM-DD")

	// ErrTransferNotEditable is returned when the caller tries to PUT a transfer row.
	// Transfers must be deleted and recreated to maintain referential integrity.
	ErrTransferNotEditable = errors.New("transfer transactions cannot be updated — delete and recreate")

	// ErrNoUpdates mirrors the accounts package error for consistency.
	ErrNoUpdates = errors.New("no updatable fields provided")
)

// ── Service interface ─────────────────────────────────────────────────────────

// Service is the business-logic contract for transactions.
type Service interface {
	Create(ctx context.Context, userID uint, req CreateTransactionRequest) (*TransactionResponse, error)
	List(ctx context.Context, userID uint, f TransactionFilter) (*TransactionListResponse, error)
	GetByID(ctx context.Context, id, userID uint) (*TransactionResponse, error)
	Update(ctx context.Context, id, userID uint, req UpdateTransactionRequest) (*TransactionResponse, error)
	Delete(ctx context.Context, id, userID uint) error
}

// ── Concrete implementation ───────────────────────────────────────────────────

// service depends on both the transactions repository (for transaction rows) and
// the accounts repository (for ownership verification and balance updates).
type service struct {
	repo        Repository
	accountRepo accounts.Repository
}

// NewService constructs the transactions service.
// accountRepo is required because every transaction modifies an account balance.
func NewService(repo Repository, accountRepo accounts.Repository) Service {
	return &service{repo: repo, accountRepo: accountRepo}
}

// ── Service method implementations ───────────────────────────────────────────

// Create inserts a transaction and adjusts the account balance atomically.
//
// Flow:
//  1. Validate type and parse date.
//  2. Verify the account belongs to the user.
//  3. For transfers, also verify the destination account.
//  4. Open a DB transaction.
//  5. Insert the row(s) and update the balance(s) inside the DB transaction.
//  6. Commit — if any step fails the entire change is rolled back.
func (s *service) Create(ctx context.Context, userID uint, req CreateTransactionRequest) (*TransactionResponse, error) {
	// Validate the transaction type.
	if !req.Type.IsValid() {
		return nil, ErrInvalidTransactionType
	}

	// Parse the business date (the day the transaction actually happened).
	date, err := time.Parse("2006-01-02", req.Date)
	if err != nil {
		return nil, ErrInvalidDate
	}

	// Verify source account ownership — user cannot post to an account they don't own.
	_, err = s.accountRepo.FindByIDAndUserID(ctx, req.AccountID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, accounts.ErrAccountNotFound
		}
		return nil, fmt.Errorf("verifying source account: %w", err)
	}

	// Handle transfer as a special multi-row operation.
	if req.Type == TypeTransfer {
		return s.createTransfer(ctx, userID, req, date)
	}

	// ── Income / Expense ──────────────────────────────────────────────────────
	t := &Transaction{
		UserID:      userID,
		AccountID:   req.AccountID,
		CategoryID:  req.CategoryID,
		Amount:      req.Amount,
		Type:        req.Type,
		Description: req.Description,
		Note:        req.Note,
		Date:        date,
	}

	// Balance delta: income adds, expense subtracts.
	delta := req.Amount
	if req.Type == TypeExpense {
		delta = -req.Amount
	}

	// Open a database transaction so the INSERT and the balance UPDATE are atomic.
	var result *Transaction
	dbErr := s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.Create(ctx, tx, t); err != nil {
			return fmt.Errorf("inserting transaction: %w", err)
		}
		if err := s.accountRepo.UpdateBalance(ctx, tx, req.AccountID, delta); err != nil {
			return fmt.Errorf("updating balance: %w", err)
		}
		result = t
		return nil
	})
	if dbErr != nil {
		return nil, dbErr
	}

	slog.Info("Transaction created", "id", result.ID, "type", result.Type, "amount", result.Amount)
	resp := toTransactionResponse(result)
	return &resp, nil
}

// createTransfer handles type=transfer by inserting two linked rows and adjusting
// two account balances — all inside a single DB transaction for atomicity.
func (s *service) createTransfer(ctx context.Context, userID uint, req CreateTransactionRequest, date time.Time) (*TransactionResponse, error) {
	if req.ToAccountID == nil {
		return nil, ErrTransferRequiresToAccount
	}
	if *req.ToAccountID == req.AccountID {
		return nil, ErrSameAccountTransfer
	}

	// Verify the destination account also belongs to the same user.
	_, err := s.accountRepo.FindByIDAndUserID(ctx, *req.ToAccountID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, accounts.ErrAccountNotFound
		}
		return nil, fmt.Errorf("verifying destination account: %w", err)
	}

	// Source leg — recorded as expense on the source account.
	source := &Transaction{
		UserID:      userID,
		AccountID:   req.AccountID,
		CategoryID:  req.CategoryID,
		Amount:      req.Amount,
		Type:        TypeExpense, // the source account loses money
		Description: req.Description,
		Note:        req.Note,
		Date:        date,
	}
	// Destination leg — recorded as income on the destination account.
	dest := &Transaction{
		UserID:      userID,
		AccountID:   *req.ToAccountID,
		CategoryID:  req.CategoryID,
		Amount:      req.Amount,
		Type:        TypeIncome, // the destination account gains money
		Description: req.Description,
		Note:        req.Note,
		Date:        date,
	}

	dbErr := s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Insert both rows (CreatePair sets up the bidirectional reference IDs).
		if err := s.repo.CreatePair(ctx, tx, source, dest); err != nil {
			return fmt.Errorf("inserting transfer pair: %w", err)
		}
		// Debit source account.
		if err := s.accountRepo.UpdateBalance(ctx, tx, req.AccountID, -req.Amount); err != nil {
			return fmt.Errorf("debiting source balance: %w", err)
		}
		// Credit destination account.
		if err := s.accountRepo.UpdateBalance(ctx, tx, *req.ToAccountID, req.Amount); err != nil {
			return fmt.Errorf("crediting destination balance: %w", err)
		}
		return nil
	})
	if dbErr != nil {
		return nil, dbErr
	}

	slog.Info("Transfer created", "source_id", source.ID, "dest_id", dest.ID, "amount", req.Amount)
	// Return the source leg as the canonical response for the caller.
	resp := toTransactionResponse(source)
	return &resp, nil
}

// List delegates filtering and pagination to the repository.
func (s *service) List(ctx context.Context, userID uint, f TransactionFilter) (*TransactionListResponse, error) {
	txs, total, err := s.repo.List(ctx, userID, f)
	if err != nil {
		return nil, fmt.Errorf("listing transactions: %w", err)
	}
	return &TransactionListResponse{
		Transactions: toTransactionResponseList(txs),
		Pagination:   BuildPagination(f.Page, f.Limit, total),
	}, nil
}

// GetByID fetches a single transaction owned by the user.
func (s *service) GetByID(ctx context.Context, id, userID uint) (*TransactionResponse, error) {
	t, err := s.repo.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTransactionNotFound
		}
		return nil, fmt.Errorf("fetching transaction: %w", err)
	}
	resp := toTransactionResponse(t)
	return &resp, nil
}

// Update patches a non-transfer transaction and corrects the account balance.
//
// Flow:
//  1. Fetch the existing row (ownership check).
//  2. Reject if it is a transfer row.
//  3. Calculate the old balance impact and the new balance impact.
//  4. Apply row changes and balance correction atomically.
func (s *service) Update(ctx context.Context, id, userID uint, req UpdateTransactionRequest) (*TransactionResponse, error) {
	existing, err := s.repo.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTransactionNotFound
		}
		return nil, fmt.Errorf("fetching transaction for update: %w", err)
	}

	// Transfers have two linked rows. Allowing partial updates would leave the
	// two legs in an inconsistent state, so we reject updates on transfer rows.
	if existing.TransferReferenceID != nil {
		return nil, ErrTransferNotEditable
	}

	// Calculate the balance delta:
	// The existing row already contributed oldDelta to the balance.
	// After the update it will contribute newDelta.
	// We add (newDelta - oldDelta) to correct the balance.
	oldAmount := existing.Amount
	newAmount := oldAmount // defaults to unchanged
	if req.Amount > 0 {
		newAmount = req.Amount
	}

	sign := 1.0 // income adds to balance
	if existing.Type == TypeExpense {
		sign = -1.0
	}
	oldDelta := sign * oldAmount
	newDelta := sign * newAmount
	balanceDiff := newDelta - oldDelta // positive = net credit, negative = net debit

	// Build the update map — only include fields that were sent.
	updates := map[string]interface{}{}
	if req.Amount > 0 {
		updates["amount"] = req.Amount
	}
	if req.CategoryID != nil {
		updates["category_id"] = req.CategoryID
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.Note != "" {
		updates["note"] = req.Note
	}
	if req.Date != "" {
		d, err := time.Parse("2006-01-02", req.Date)
		if err != nil {
			return nil, ErrInvalidDate
		}
		updates["date"] = d
	}

	if len(updates) == 0 {
		return nil, ErrNoUpdates
	}

	dbErr := s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.Update(ctx, tx, id, userID, updates); err != nil {
			return fmt.Errorf("updating transaction row: %w", err)
		}
		// Only touch the balance if the amount changed.
		if balanceDiff != 0 {
			if err := s.accountRepo.UpdateBalance(ctx, tx, existing.AccountID, balanceDiff); err != nil {
				return fmt.Errorf("correcting balance: %w", err)
			}
		}
		return nil
	})
	if dbErr != nil {
		return nil, dbErr
	}

	return s.GetByID(ctx, id, userID)
}

// Delete soft-deletes a transaction and reverses its balance impact.
//
// For transfer rows: both legs are deleted and both account balances are reversed.
func (s *service) Delete(ctx context.Context, id, userID uint) error {
	existing, err := s.repo.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTransactionNotFound
		}
		return fmt.Errorf("fetching transaction for delete: %w", err)
	}

	// Calculate the balance reversal for the primary row.
	primaryDelta := balanceDelta(existing.Type, existing.Amount)
	// Reverse it (negate) — we are undoing the original effect.
	primaryReversal := -primaryDelta

	return s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Soft-delete the primary row.
		if err := s.repo.Delete(ctx, tx, id, userID); err != nil {
			return fmt.Errorf("deleting transaction: %w", err)
		}
		// Reverse this row's balance impact.
		if err := s.accountRepo.UpdateBalance(ctx, tx, existing.AccountID, primaryReversal); err != nil {
			return fmt.Errorf("reversing primary balance: %w", err)
		}

		// If this is one leg of a transfer, also delete the linked leg.
		if existing.TransferReferenceID != nil {
			linked, err := s.repo.FindLinkedTransfer(ctx, *existing.TransferReferenceID)
			if err != nil {
				return fmt.Errorf("fetching linked transfer: %w", err)
			}
			if linked != nil {
				linkedReversal := -balanceDelta(linked.Type, linked.Amount)
				// Delete the linked row (it belongs to the same user so userID is safe).
				if err := s.repo.Delete(ctx, tx, linked.ID, userID); err != nil {
					return fmt.Errorf("deleting linked transfer leg: %w", err)
				}
				if err := s.accountRepo.UpdateBalance(ctx, tx, linked.AccountID, linkedReversal); err != nil {
					return fmt.Errorf("reversing linked balance: %w", err)
				}
			}
		}

		slog.Info("Transaction deleted", "id", id, "user_id", userID)
		return nil
	})
}

// ── Private helpers ────────────────────────────────────────────────────────────

// balanceDelta returns how much a transaction of the given type and amount
// contributed to the account balance (positive = credit, negative = debit).
func balanceDelta(t TransactionType, amount float64) float64 {
	if t == TypeExpense {
		return -amount
	}
	return amount // income and (as stored) transfer legs are always +amount
}
