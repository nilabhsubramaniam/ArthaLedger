package reports

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ── Sentinel errors ────────────────────────────────────────────────────────────

var (
	// ErrInvalidYear is returned when the year parameter is outside 2000–2100.
	ErrInvalidYear = errors.New("year must be between 2000 and 2100")

	// ErrInvalidMonth is returned when the month parameter is not 1–12.
	ErrInvalidMonth = errors.New("month must be between 1 and 12")

	// ErrInvalidMonths is returned when the trends window is outside 1–24 months.
	ErrInvalidMonths = errors.New("months must be between 1 and 24")

	// ErrFutureMonth is returned when the requested month is in the future.
	ErrFutureMonth = errors.New("cannot generate a report for a future month")
)

// ── Service interface ──────────────────────────────────────────────────────────

// Service is the business-logic contract for the reports domain.
// All operations are read-only; the service never writes data.
type Service interface {
	// GetMonthlySummary returns income, expense, net savings, and category
	// breakdown for the given calendar month.
	GetMonthlySummary(ctx context.Context, userID uint, year, month int) (*MonthlySummary, error)

	// GetTrend returns month-by-month income/expense/net for the last N months.
	GetTrend(ctx context.Context, userID uint, months int) (*TrendResponse, error)

	// GetExportRows returns raw transaction rows for a month, ready for CSV streaming.
	GetExportRows(ctx context.Context, userID uint, year, month int) ([]ExportRow, error)
}

// ── Concrete implementation ────────────────────────────────────────────────────

type service struct {
	repo Repository
}

// NewService constructs the reports service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// ── Method implementations ─────────────────────────────────────────────────────

// GetMonthlySummary validates the parameters and delegates to the repository.
//
// Business rules:
//   - year must be in [2000, 2100].
//   - month must be in [1, 12].
//   - The requested month must not be in the future.
func (s *service) GetMonthlySummary(ctx context.Context, userID uint, year, month int) (*MonthlySummary, error) {
	if err := validateYearMonth(year, month); err != nil {
		return nil, err
	}

	summary, err := s.repo.GetMonthlySummary(ctx, userID, year, month)
	if err != nil {
		return nil, fmt.Errorf("monthly summary query: %w", err)
	}
	return summary, nil
}

// GetTrend validates months and delegates to the repository.
//
// Business rules:
//   - months must be in [1, 24].
func (s *service) GetTrend(ctx context.Context, userID uint, months int) (*TrendResponse, error) {
	if months < 1 || months > 24 {
		return nil, ErrInvalidMonths
	}

	points, err := s.repo.GetTrend(ctx, userID, months)
	if err != nil {
		return nil, fmt.Errorf("trend query: %w", err)
	}

	return &TrendResponse{
		Months: months,
		Points: points,
	}, nil
}

// GetExportRows validates the parameters and returns rows for CSV streaming.
func (s *service) GetExportRows(ctx context.Context, userID uint, year, month int) ([]ExportRow, error) {
	if err := validateYearMonth(year, month); err != nil {
		return nil, err
	}

	rows, err := s.repo.GetExportRows(ctx, userID, year, month)
	if err != nil {
		return nil, fmt.Errorf("export query: %w", err)
	}
	return rows, nil
}

// ── Private helpers ────────────────────────────────────────────────────────────

// validateYearMonth checks that year/month are in range and not in the future.
func validateYearMonth(year, month int) error {
	if year < 2000 || year > 2100 {
		return ErrInvalidYear
	}
	if month < 1 || month > 12 {
		return ErrInvalidMonth
	}
	now := time.Now().UTC()
	if year > now.Year() || (year == now.Year() && month > int(now.Month())) {
		return ErrFutureMonth
	}
	return nil
}
