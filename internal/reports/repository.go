package reports

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ── Interface ──────────────────────────────────────────────────────────────────

// Repository is the data-access contract for the reports package.
// All methods are read-only; no data is ever written by the reports package.
type Repository interface {
	// GetMonthlySummary returns aggregated income, expense, and per-category
	// expense breakdown for the given user / year / month.
	GetMonthlySummary(ctx context.Context, userID uint, year, month int) (*MonthlySummary, error)

	// GetTrend returns one TrendPoint per calendar month for the last `months`
	// months (including the current partial month), ordered oldest → newest.
	GetTrend(ctx context.Context, userID uint, months int) ([]TrendPoint, error)

	// GetExportRows returns all non-deleted transactions for the given month,
	// joined with category and account names for the CSV export.
	GetExportRows(ctx context.Context, userID uint, year, month int) ([]ExportRow, error)
}

// ── PostgreSQL implementation ──────────────────────────────────────────────────

type postgresRepository struct {
	db *gorm.DB
}

// NewRepository returns a reports Repository backed by the given GORM handle.
func NewRepository(db *gorm.DB) Repository {
	return &postgresRepository{db: db}
}

// GetMonthlySummary runs three queries:
//  1. Sum of income for the month.
//  2. Sum of expense for the month.
//  3. Expense grouped by category (with category name via LEFT JOIN).
//
// All queries are scoped to the caller's user_id and the given year/month.
func (r *postgresRepository) GetMonthlySummary(
	ctx context.Context,
	userID uint,
	year, month int,
) (*MonthlySummary, error) {

	// ── Income total ─────────────────────────────────────────────────────────
	var totalIncome float64
	if err := r.db.WithContext(ctx).
		Table("transactions").
		Where(
			"user_id = ? AND type = 'income' AND deleted_at IS NULL AND EXTRACT(YEAR FROM date) = ? AND EXTRACT(MONTH FROM date) = ?",
			userID, year, month,
		).
		Select("COALESCE(SUM(amount), 0)").
		Row().Scan(&totalIncome); err != nil {
		return nil, err
	}

	// ── Expense total ─────────────────────────────────────────────────────────
	var totalExpense float64
	if err := r.db.WithContext(ctx).
		Table("transactions").
		Where(
			"user_id = ? AND type = 'expense' AND deleted_at IS NULL AND EXTRACT(YEAR FROM date) = ? AND EXTRACT(MONTH FROM date) = ?",
			userID, year, month,
		).
		Select("COALESCE(SUM(amount), 0)").
		Row().Scan(&totalExpense); err != nil {
		return nil, err
	}

	// ── Expense by category ───────────────────────────────────────────────────
	// LEFT JOIN brings in the category name; NULL category_id returns "Uncategorized".
	type catRow struct {
		CategoryID   *uint
		CategoryName *string
		Amount       float64
		Count        int64
	}

	rows, err := r.db.WithContext(ctx).
		Table("transactions t").
		Select(`
			t.category_id,
			c.name  AS category_name,
			COALESCE(SUM(t.amount), 0) AS amount,
			COUNT(*) AS count
		`).
		Joins("LEFT JOIN categories c ON c.id = t.category_id").
		Where(
			"t.user_id = ? AND t.type = 'expense' AND t.deleted_at IS NULL AND EXTRACT(YEAR FROM t.date) = ? AND EXTRACT(MONTH FROM t.date) = ?",
			userID, year, month,
		).
		Group("t.category_id, c.name").
		Order("amount DESC").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var breakdown []CategoryBreakdown
	for rows.Next() {
		var cr catRow
		if err := rows.Scan(&cr.CategoryID, &cr.CategoryName, &cr.Amount, &cr.Count); err != nil {
			return nil, err
		}
		name := "Uncategorized"
		if cr.CategoryName != nil {
			name = *cr.CategoryName
		}
		var pct float64
		if totalExpense > 0 {
			pct = (cr.Amount / totalExpense) * 100
		}
		breakdown = append(breakdown, CategoryBreakdown{
			CategoryID:       cr.CategoryID,
			CategoryName:     name,
			Amount:           cr.Amount,
			Percentage:       roundTwo(pct),
			TransactionCount: cr.Count,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &MonthlySummary{
		Year:         year,
		Month:        month,
		TotalIncome:  roundTwo(totalIncome),
		TotalExpense: roundTwo(totalExpense),
		NetSavings:   roundTwo(totalIncome - totalExpense),
		ByCategory:   breakdown,
	}, nil
}

// GetTrend returns one TrendPoint per calendar month for the last N months.
// The window starts at the first day of (today − months + 1) and runs to today.
func (r *postgresRepository) GetTrend(ctx context.Context, userID uint, months int) ([]TrendPoint, error) {
	// Calculate the start date: first day of the earliest month in the window.
	now := time.Now().UTC()
	startMonth := now.AddDate(0, -(months - 1), 0)
	windowStart := time.Date(startMonth.Year(), startMonth.Month(), 1, 0, 0, 0, 0, time.UTC)

	type trendRow struct {
		Year    int
		Month   int
		Income  float64
		Expense float64
	}

	rows, err := r.db.WithContext(ctx).
		Table("transactions").
		Select(`
			EXTRACT(YEAR  FROM date)::int AS year,
			EXTRACT(MONTH FROM date)::int AS month,
			COALESCE(SUM(CASE WHEN type = 'income'  THEN amount ELSE 0 END), 0) AS income,
			COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0) AS expense
		`).
		Where(
			"user_id = ? AND type IN ('income', 'expense') AND deleted_at IS NULL AND date >= ?",
			userID, windowStart.Format("2006-01-02"),
		).
		Group("year, month").
		Order("year ASC, month ASC").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []TrendPoint
	for rows.Next() {
		var tr trendRow
		if err := rows.Scan(&tr.Year, &tr.Month, &tr.Income, &tr.Expense); err != nil {
			return nil, err
		}
		points = append(points, TrendPoint{
			Year:    tr.Year,
			Month:   tr.Month,
			Income:  roundTwo(tr.Income),
			Expense: roundTwo(tr.Expense),
			Net:     roundTwo(tr.Income - tr.Expense),
		})
	}

	return points, rows.Err()
}

// GetExportRows returns all transactions for a month joined with category and
// account names. The result is used by the handler to stream a CSV response.
func (r *postgresRepository) GetExportRows(
	ctx context.Context,
	userID uint,
	year, month int,
) ([]ExportRow, error) {

	type rawRow struct {
		Date        time.Time
		Description string
		Amount      float64
		Type        string
		Category    *string
		AccountName string
		Note        string
	}

	rows, err := r.db.WithContext(ctx).
		Table("transactions t").
		Select(`
			t.date,
			t.description,
			t.amount,
			t.type,
			c.name  AS category,
			a.name  AS account_name,
			t.note
		`).
		Joins("LEFT JOIN categories c ON c.id = t.category_id").
		Joins("LEFT JOIN accounts   a ON a.id = t.account_id").
		Where(
			"t.user_id = ? AND t.deleted_at IS NULL AND EXTRACT(YEAR FROM t.date) = ? AND EXTRACT(MONTH FROM t.date) = ?",
			userID, year, month,
		).
		Order("t.date ASC, t.id ASC").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ExportRow
	for rows.Next() {
		var r rawRow
		if err := rows.Scan(&r.Date, &r.Description, &r.Amount, &r.Type, &r.Category, &r.AccountName, &r.Note); err != nil {
			return nil, err
		}
		cat := ""
		if r.Category != nil {
			cat = *r.Category
		}
		result = append(result, ExportRow{
			Date:        r.Date,
			Description: r.Description,
			Amount:      r.Amount,
			Type:        r.Type,
			Category:    cat,
			AccountName: r.AccountName,
			Note:        r.Note,
		})
	}

	return result, rows.Err()
}

// ── Helpers ────────────────────────────────────────────────────────────────────

// roundTwo rounds a float64 to two decimal places.
func roundTwo(v float64) float64 {
	// Multiply, truncate to int, divide — avoids importing math for a simple rounding op.
	// We use integer arithmetic to avoid floating-point drift from math.Round.
	shifted := v * 100
	if shifted < 0 {
		shifted -= 0.5
	} else {
		shifted += 0.5
	}
	return float64(int64(shifted)) / 100
}
