package reports

import "time"

// ── Monthly Summary ────────────────────────────────────────────────────────────

// MonthlySummary is the top-level response for GET /reports/monthly.
// It contains income, expenses, and net savings for one calendar month,
// plus a per-category expense breakdown.
type MonthlySummary struct {
	Year  int `json:"year"`
	Month int `json:"month"`

	// TotalIncome is the sum of all income transactions in the month.
	TotalIncome float64 `json:"total_income"`

	// TotalExpense is the sum of all expense transactions in the month.
	TotalExpense float64 `json:"total_expense"`

	// NetSavings = TotalIncome − TotalExpense. Negative = deficit.
	NetSavings float64 `json:"net_savings"`

	// ByCategory breaks expenses down by category, sorted by amount descending.
	ByCategory []CategoryBreakdown `json:"by_category"`
}

// CategoryBreakdown is one row in the ByCategory expense breakdown.
type CategoryBreakdown struct {
	// CategoryID is nil when the transaction has no category assigned.
	CategoryID   *uint   `json:"category_id"`
	CategoryName string  `json:"category_name"` // "Uncategorized" when category_id is nil
	Amount       float64 `json:"amount"`

	// Percentage is this category's share of TotalExpense (0‒100).
	Percentage float64 `json:"percentage"`

	// TransactionCount is the number of expense transactions contributing.
	TransactionCount int64 `json:"transaction_count"`
}

// ── Trend Data ─────────────────────────────────────────────────────────────────

// TrendResponse wraps the trend-point slice with request metadata.
type TrendResponse struct {
	// Months is the number of calendar months covered by the trend.
	Months int `json:"months"`

	// Points contains one entry per calendar month, ordered oldest → newest.
	Points []TrendPoint `json:"points"`
}

// TrendPoint holds income / expense / net figures for one calendar month.
type TrendPoint struct {
	Year    int     `json:"year"`
	Month   int     `json:"month"`
	Income  float64 `json:"income"`
	Expense float64 `json:"expense"`

	// Net = Income − Expense.
	Net float64 `json:"net"`
}

// ── CSV Export (internal) ──────────────────────────────────────────────────────

// ExportRow is a single row in the CSV export. It is never serialised to JSON;
// it exists only as an intermediate type used by the repository → handler pipeline.
type ExportRow struct {
	Date        time.Time
	Description string
	Amount      float64
	Type        string
	Category    string // empty when uncategorized
	AccountName string
	Note        string
}
