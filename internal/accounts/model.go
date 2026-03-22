// Package accounts manages financial accounts belonging to a user.
// An account represents any financial vessel: bank account, cash wallet,
// credit card, or investment portfolio.
//
// Layer responsibilities in this package:
//
//	model.go      — GORM struct, type enums, request/response types
//	repository.go — database queries (interface + PostgreSQL impl)
//	service.go    — business rules (ownership, balance immutability, etc.)
//	handler.go    — HTTP handlers wired to Gin routes with Swagger annotations
package accounts

import (
	"time"

	"gorm.io/gorm"
)

// ── Type enum ──────────────────────────────────────────────────────────────────

// AccountType represents the category of a financial account.
// Stored as a VARCHAR(50) in PostgreSQL; validated in the service layer.
type AccountType string

const (
	AccountTypeBank       AccountType = "bank"        // savings or current bank account
	AccountTypeCash       AccountType = "cash"         // physical cash wallet
	AccountTypeCreditCard AccountType = "credit_card"  // credit card (balance can be negative)
	AccountTypeInvestment AccountType = "investment"   // stocks, mutual funds, FD
)

// IsValid returns true when the AccountType is one of the four allowed values.
// Called by the service before persisting to prevent invalid data in the DB.
func (t AccountType) IsValid() bool {
	switch t {
	case AccountTypeBank, AccountTypeCash, AccountTypeCreditCard, AccountTypeInvestment:
		return true
	}
	return false
}

// ── Database model ─────────────────────────────────────────────────────────────

// Account is the GORM model mapping to the `accounts` table (created in 000001).
// Balance is managed exclusively by the transactions service — the account
// update endpoint deliberately does not expose a balance field.
type Account struct {
	ID        uint            `gorm:"primaryKey"                                           json:"id"`
	UserID    uint            `gorm:"not null;index"                                       json:"user_id"`
	Name      string          `gorm:"type:varchar(255);not null"                           json:"name"`
	Type      AccountType     `gorm:"type:varchar(50);not null"                            json:"type"`
	Balance   float64         `gorm:"type:numeric(15,2);not null;default:0"                json:"balance"`
	Currency  string          `gorm:"type:varchar(10);not null;default:'INR'"              json:"currency"`
	IsActive  bool            `gorm:"not null;default:true"                                json:"is_active"`
	CreatedAt time.Time       `                                                            json:"created_at"`
	UpdatedAt time.Time       `                                                            json:"updated_at"`
	DeletedAt gorm.DeletedAt  `gorm:"index"                                                json:"-"` // soft-delete — never expose
}

// ── HTTP request types ─────────────────────────────────────────────────────────

// CreateAccountRequest is the JSON body for POST /accounts.
// Initial balance can be set at creation (e.g. capturing an existing bank balance).
// After creation, balance is controlled only by transactions.
type CreateAccountRequest struct {
	Name           string      `json:"name"            binding:"required,min=1,max=255"`
	Type           AccountType `json:"type"            binding:"required"`
	Currency       string      `json:"currency"        binding:"required,min=2,max=10"`
	InitialBalance float64     `json:"initial_balance"` // optional; defaults to 0
}

// UpdateAccountRequest is the JSON body for PUT /accounts/:id.
// Balance and currency are intentionally excluded:
//   - Balance is a derived value managed by transactions.
//   - Currency is immutable once transactions exist (changing it would
//     invalidate historical amounts).
type UpdateAccountRequest struct {
	Name     string      `json:"name"      binding:"omitempty,min=1,max=255"`
	Type     AccountType `json:"type"      binding:"omitempty"`
	IsActive *bool       `json:"is_active"` // pointer so we can detect "not sent" vs false
}

// ── HTTP response types ────────────────────────────────────────────────────────

// AccountResponse is the public view of an Account returned in all single-item responses.
type AccountResponse struct {
	ID        uint        `json:"id"`
	UserID    uint        `json:"user_id"`
	Name      string      `json:"name"`
	Type      AccountType `json:"type"`
	Balance   float64     `json:"balance"`
	Currency  string      `json:"currency"`
	IsActive  bool        `json:"is_active"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// AccountListResponse wraps a slice of AccountResponse with a count,
// allowing callers to get the total without a separate API call.
type AccountListResponse struct {
	Accounts []AccountResponse `json:"accounts"`
	Total    int64             `json:"total"`
}

// AccountSummaryResponse enriches AccountResponse with derived statistics
// computed by joining with the transactions table.
type AccountSummaryResponse struct {
	Account             AccountResponse `json:"account"`
	TransactionCount    int64           `json:"transaction_count"`
	LastTransactionDate *time.Time      `json:"last_transaction_date"` // nil when no transactions
}

// ── Conversion helpers ─────────────────────────────────────────────────────────

// toAccountResponse converts an Account model to its API-safe representation.
// Centralised here so no handler has to manually map fields.
func toAccountResponse(a *Account) AccountResponse {
	return AccountResponse{
		ID:        a.ID,
		UserID:    a.UserID,
		Name:      a.Name,
		Type:      a.Type,
		Balance:   a.Balance,
		Currency:  a.Currency,
		IsActive:  a.IsActive,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

// toAccountResponseList converts a slice of Account models.
func toAccountResponseList(accounts []Account) []AccountResponse {
	result := make([]AccountResponse, len(accounts))
	for i := range accounts {
		result[i] = toAccountResponse(&accounts[i])
	}
	return result
}
