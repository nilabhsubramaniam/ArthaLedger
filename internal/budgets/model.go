package budgets

import (
	"time"

	"gorm.io/gorm"
)

// ── Period enum ────────────────────────────────────────────────────────────────

// BudgetPeriod is the recurrence window for a budget.
// Stored as VARCHAR(20) in PostgreSQL; validated by the service layer.
type BudgetPeriod string

const (
	PeriodDaily   BudgetPeriod = "daily"
	PeriodWeekly  BudgetPeriod = "weekly"
	PeriodMonthly BudgetPeriod = "monthly"
	PeriodYearly  BudgetPeriod = "yearly"
)

// IsValid returns true for the four defined period values.
func (p BudgetPeriod) IsValid() bool {
	switch p {
	case PeriodDaily, PeriodWeekly, PeriodMonthly, PeriodYearly:
		return true
	}
	return false
}

// ── Database model ─────────────────────────────────────────────────────────────

// Budget is the GORM model mapping to the `budgets` table (created in 000001).
//
// The "amount" column is the spending limit. Actual spend is never stored here;
// it is computed on-the-fly by the repository querying the transactions table.
// This keeps the source of truth centralised in the transactions table.
type Budget struct {
	ID     uint `gorm:"primaryKey"                          json:"id"`
	UserID uint `gorm:"not null;index"                      json:"user_id"`

	// CategoryID scopes the budget to one spending category.
	// When nil the budget tracks all expense categories combined.
	CategoryID *uint `gorm:"index"                               json:"category_id"`

	Name   string       `gorm:"type:varchar(255);not null"          json:"name"`
	Amount float64      `gorm:"type:numeric(15,2);not null"         json:"amount"`
	Period BudgetPeriod `gorm:"type:varchar(20);not null"           json:"period"`

	// StartDate marks the beginning of the first recurring window.
	StartDate time.Time `gorm:"type:date;not null"                  json:"start_date"`

	// EndDate optionally caps the budget lifetime. nil = open-ended.
	EndDate *time.Time `gorm:"type:date"                           json:"end_date"`

	IsActive  bool      `gorm:"not null;default:true"               json:"is_active"`
	CreatedAt time.Time `                                           json:"created_at"`
	UpdatedAt time.Time `                                           json:"updated_at"`

	// Soft-delete: GORM sets deleted_at instead of removing the row.
	DeletedAt gorm.DeletedAt `gorm:"index"                               json:"-"`
}

// TableName overrides GORM table-name inference.
func (Budget) TableName() string { return "budgets" }

// ── Request types ──────────────────────────────────────────────────────────────

// CreateBudgetRequest is the JSON body for POST /budgets.
type CreateBudgetRequest struct {
	// Name is a human-readable label for the budget (e.g. "Monthly Groceries").
	Name string `json:"name" binding:"required,min=1,max=255"`

	// CategoryID restricts spending tracking to one category.
	// Omit (or set null) to track all expense categories combined.
	CategoryID *uint `json:"category_id"`

	// Amount is the maximum allowed spend in the account's default currency.
	Amount float64 `json:"amount" binding:"required,gt=0"`

	// Period is the recurrence window: daily | weekly | monthly | yearly.
	Period BudgetPeriod `json:"period" binding:"required"`

	// StartDate is the beginning of the first period window, in "YYYY-MM-DD" format.
	StartDate string `json:"start_date" binding:"required"`

	// EndDate optionally caps the budget lifetime ("YYYY-MM-DD"). Omit for open-ended.
	EndDate string `json:"end_date"`
}

// UpdateBudgetRequest is the JSON body for PUT /budgets/:id.
// All fields are optional; only non-zero values are applied.
type UpdateBudgetRequest struct {
	// Name replaces the budget label.
	Name string `json:"name" binding:"omitempty,min=1,max=255"`

	// Amount replaces the spending limit (must be > 0 if provided).
	Amount float64 `json:"amount" binding:"omitempty,gt=0"`

	// Period replaces the recurrence window.
	Period BudgetPeriod `json:"period"`

	// EndDate replaces the expiry date. Send an empty string to clear it.
	EndDate string `json:"end_date"`

	// IsActive toggles the budget on or off. Use a pointer so "false" can be
	// distinguished from "not provided".
	IsActive *bool `json:"is_active"`
}

// ── Response types ─────────────────────────────────────────────────────────────

// BudgetResponse is the JSON shape returned to the client for a single budget.
// It enriches the raw Budget row with live Spent / Remaining / PercentUsed
// values computed from the transactions table.
type BudgetResponse struct {
	ID         uint         `json:"id"`
	UserID     uint         `json:"user_id"`
	CategoryID *uint        `json:"category_id"`
	Name       string       `json:"name"`
	Amount     float64      `json:"amount"`
	Period     BudgetPeriod `json:"period"`
	StartDate  time.Time    `json:"start_date"`
	EndDate    *time.Time   `json:"end_date,omitempty"`
	IsActive   bool         `json:"is_active"`

	// Spent is the total expenses charged against this budget in the current period.
	// Computed at query time — never stored in the DB.
	Spent float64 `json:"spent"`

	// Remaining is Amount − Spent. Negative when the budget is exceeded.
	Remaining float64 `json:"remaining"`

	// PercentUsed is (Spent / Amount) × 100, rounded to two decimal places.
	// Capped at 100 in the response value (but Spent may still exceed Amount).
	PercentUsed float64 `json:"percent_used"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BudgetListResponse wraps a list of budget responses with a total count.
type BudgetListResponse struct {
	Budgets []BudgetResponse `json:"budgets"`
	Total   int64            `json:"total"`
}
