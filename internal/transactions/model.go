// Package transactions manages financial transaction records.
// Each transaction belongs to an account and adjusts that account's balance.
// Transfers create two linked rows — one expense on the source account and
// one income on the destination account.
//
// Layer responsibilities:
//
//	model.go      — GORM struct, type enums, request/response/filter types
//	repository.go — database queries with filtering and pagination
//	service.go    — business logic: balance adjustment, transfer pairs, ownership
//	handler.go    — Gin HTTP handlers with Swagger annotations
package transactions

import (
	"time"

	"gorm.io/gorm"
)

// ── Type enums ────────────────────────────────────────────────────────────────

// TransactionType classifies the direction of money flow.
type TransactionType string

const (
	TypeIncome   TransactionType = "income"   // money in (salary, freelance)
	TypeExpense  TransactionType = "expense"  // money out (groceries, rent)
	TypeTransfer TransactionType = "transfer" // move between two of the user's own accounts
)

// IsValid returns true for the three recognised transaction types.
func (t TransactionType) IsValid() bool {
	switch t {
	case TypeIncome, TypeExpense, TypeTransfer:
		return true
	}
	return false
}

// ── Database model ────────────────────────────────────────────────────────────

// Transaction is the GORM model mapping to the `transactions` table.
//
// Amount is always stored as a positive value — the Type column determines
// whether it increases or decreases the account balance:
//   income  → balance += amount
//   expense → balance -= amount
//   transfer → handled as two rows; see service.go
//
// TransferReferenceID links the two rows of a transfer pair together
// (added by migration 000002). Nil for income/expense rows.
type Transaction struct {
	ID                  uint            `gorm:"primaryKey"                             json:"id"`
	UserID              uint            `gorm:"not null;index"                         json:"user_id"`
	AccountID           uint            `gorm:"not null;index"                         json:"account_id"`
	CategoryID          *uint           `gorm:"index"                                  json:"category_id"`  // nullable
	Amount              float64         `gorm:"type:numeric(15,2);not null"            json:"amount"`
	Type                TransactionType `gorm:"type:varchar(20);not null"              json:"type"`
	Description         string          `gorm:"type:text"                              json:"description"`
	Note                string          `gorm:"type:text"                              json:"note"`
	Date                time.Time       `gorm:"type:date;not null"                     json:"date"`
	TransferReferenceID *uint           `gorm:"index"                                  json:"transfer_reference_id"` // links transfer pair
	CreatedAt           time.Time       `                                              json:"created_at"`
	UpdatedAt           time.Time       `                                              json:"updated_at"`
	DeletedAt           gorm.DeletedAt  `gorm:"index"                                  json:"-"` // soft-delete
}

// ── HTTP request types ────────────────────────────────────────────────────────

// CreateTransactionRequest is the JSON body for POST /transactions.
//
// For transfers, ToAccountID must be provided. Amount must always be positive.
// CategoryID is optional — if omitted, the auto-categoriser will run (Phase 5).
type CreateTransactionRequest struct {
	AccountID   uint            `json:"account_id"   binding:"required"`
	ToAccountID *uint           `json:"to_account_id"`                    // only for type=transfer
	CategoryID  *uint           `json:"category_id"`                      // optional
	Amount      float64         `json:"amount"       binding:"required,gt=0"` // must be > 0
	Type        TransactionType `json:"type"         binding:"required"`
	Description string          `json:"description"  binding:"required,max=500"`
	Note        string          `json:"note"         binding:"omitempty,max=1000"`
	Date        string          `json:"date"         binding:"required"` // "YYYY-MM-DD"
}

// UpdateTransactionRequest is the JSON body for PUT /transactions/:id.
// Transfer transactions cannot be updated — delete and recreate instead.
// Amount, CategoryID, Description, Note, and Date are patchable.
type UpdateTransactionRequest struct {
	CategoryID  *uint   `json:"category_id"`
	Amount      float64 `json:"amount"      binding:"omitempty,gt=0"`
	Description string  `json:"description" binding:"omitempty,max=500"`
	Note        string  `json:"note"        binding:"omitempty,max=1000"`
	Date        string  `json:"date"`        // "YYYY-MM-DD", optional
}

// ── Filter / pagination types ─────────────────────────────────────────────────

// TransactionFilter is populated from the GET /transactions query parameters.
// All fields are optional — omitting a field means "do not filter by it".
type TransactionFilter struct {
	AccountID  *uint           // ?account_id=3
	CategoryID *uint           // ?category_id=7
	Type       TransactionType // ?type=expense
	DateFrom   *time.Time      // ?date_from=2026-01-01
	DateTo     *time.Time      // ?date_to=2026-03-31
	MinAmount  *float64        // ?min_amount=500
	MaxAmount  *float64        // ?max_amount=5000
	Page       int             // ?page=1     (default 1)
	Limit      int             // ?limit=20   (default 20, max 100)
}

// Pagination holds the metadata returned alongside every paginated list.
type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// ── HTTP response types ────────────────────────────────────────────────────────

// TransactionResponse is the API-safe representation of a Transaction.
type TransactionResponse struct {
	ID                  uint            `json:"id"`
	UserID              uint            `json:"user_id"`
	AccountID           uint            `json:"account_id"`
	CategoryID          *uint           `json:"category_id"`
	Amount              float64         `json:"amount"`
	Type                TransactionType `json:"type"`
	Description         string          `json:"description"`
	Note                string          `json:"note"`
	Date                time.Time       `json:"date"`
	TransferReferenceID *uint           `json:"transfer_reference_id"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// TransactionListResponse wraps a paginated slice of TransactionResponse.
type TransactionListResponse struct {
	Transactions []TransactionResponse `json:"transactions"`
	Pagination   Pagination            `json:"pagination"`
}

// ── Conversion helpers ────────────────────────────────────────────────────────

// toTransactionResponse converts a Transaction model to its API representation.
func toTransactionResponse(t *Transaction) TransactionResponse {
	return TransactionResponse{
		ID:                  t.ID,
		UserID:              t.UserID,
		AccountID:           t.AccountID,
		CategoryID:          t.CategoryID,
		Amount:              t.Amount,
		Type:                t.Type,
		Description:         t.Description,
		Note:                t.Note,
		Date:                t.Date,
		TransferReferenceID: t.TransferReferenceID,
		CreatedAt:           t.CreatedAt,
		UpdatedAt:           t.UpdatedAt,
	}
}

// toTransactionResponseList converts a slice of Transaction models.
func toTransactionResponseList(txs []Transaction) []TransactionResponse {
	result := make([]TransactionResponse, len(txs))
	for i := range txs {
		result[i] = toTransactionResponse(&txs[i])
	}
	return result
}
