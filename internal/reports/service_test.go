package reports_test

// Unit tests for the reports service layer.
//
// The service delegates directly to the repository once validation passes.
// All interesting logic is in validateYearMonth / bounds checking, which is
// what we test here.  Repository calls are mocked so these tests remain
// independent of any database.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nilabh/arthaledger/internal/reports"
)

// ── Mock repository ───────────────────────────────────────────────────────────

type mockReportRepo struct {
	getMonthlySummaryFn func(ctx context.Context, userID uint, year, month int) (*reports.MonthlySummary, error)
	getTrendFn          func(ctx context.Context, userID uint, months int) ([]reports.TrendPoint, error)
	getExportRowsFn     func(ctx context.Context, userID uint, year, month int) ([]reports.ExportRow, error)
}

func (m *mockReportRepo) GetMonthlySummary(ctx context.Context, userID uint, year, month int) (*reports.MonthlySummary, error) {
	if m.getMonthlySummaryFn != nil {
		return m.getMonthlySummaryFn(ctx, userID, year, month)
	}
	return &reports.MonthlySummary{Year: year, Month: month}, nil
}

func (m *mockReportRepo) GetTrend(ctx context.Context, userID uint, months int) ([]reports.TrendPoint, error) {
	if m.getTrendFn != nil {
		return m.getTrendFn(ctx, userID, months)
	}
	return []reports.TrendPoint{}, nil
}

func (m *mockReportRepo) GetExportRows(ctx context.Context, userID uint, year, month int) ([]reports.ExportRow, error) {
	if m.getExportRowsFn != nil {
		return m.getExportRowsFn(ctx, userID, year, month)
	}
	return []reports.ExportRow{}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// pastYearMonth returns a year/month that is guaranteed to be in the past
// (avoids brittle hard-coded dates that become "future" over time).
func pastYearMonth() (int, int) {
	now := time.Now().UTC()
	// Use the previous month to stay safely in the past.
	prev := now.AddDate(0, -1, 0)
	return prev.Year(), int(prev.Month())
}

// ── GetMonthlySummary tests ───────────────────────────────────────────────────

func TestReportService_GetMonthlySummary_Success(t *testing.T) {
	t.Parallel()
	year, month := pastYearMonth()
	svc := reports.NewService(&mockReportRepo{})

	summary, err := svc.GetMonthlySummary(context.Background(), 1, year, month)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if summary.Year != year {
		t.Errorf("year: got %d, want %d", summary.Year, year)
	}
	if summary.Month != month {
		t.Errorf("month: got %d, want %d", summary.Month, month)
	}
}

func TestReportService_GetMonthlySummary_InvalidYear_TooLow(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	_, err := svc.GetMonthlySummary(context.Background(), 1, 1999, 6)
	if !errors.Is(err, reports.ErrInvalidYear) {
		t.Errorf("expected ErrInvalidYear, got %v", err)
	}
}

func TestReportService_GetMonthlySummary_InvalidYear_TooHigh(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	_, err := svc.GetMonthlySummary(context.Background(), 1, 2101, 6)
	if !errors.Is(err, reports.ErrInvalidYear) {
		t.Errorf("expected ErrInvalidYear, got %v", err)
	}
}

func TestReportService_GetMonthlySummary_InvalidMonth_Zero(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	_, err := svc.GetMonthlySummary(context.Background(), 1, 2024, 0)
	if !errors.Is(err, reports.ErrInvalidMonth) {
		t.Errorf("expected ErrInvalidMonth, got %v", err)
	}
}

func TestReportService_GetMonthlySummary_InvalidMonth_Thirteen(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	_, err := svc.GetMonthlySummary(context.Background(), 1, 2024, 13)
	if !errors.Is(err, reports.ErrInvalidMonth) {
		t.Errorf("expected ErrInvalidMonth, got %v", err)
	}
}

func TestReportService_GetMonthlySummary_FutureMonth(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	futureYear := time.Now().UTC().Year() + 1
	_, err := svc.GetMonthlySummary(context.Background(), 1, futureYear, 1)
	if !errors.Is(err, reports.ErrFutureMonth) {
		t.Errorf("expected ErrFutureMonth, got %v", err)
	}
}

// ── GetTrend tests ────────────────────────────────────────────────────────────

func TestReportService_GetTrend_Success(t *testing.T) {
	t.Parallel()
	repo := &mockReportRepo{
		getTrendFn: func(_ context.Context, _ uint, months int) ([]reports.TrendPoint, error) {
			points := make([]reports.TrendPoint, months)
			return points, nil
		},
	}
	svc := reports.NewService(repo)

	trend, err := svc.GetTrend(context.Background(), 1, 6)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if trend.Months != 6 {
		t.Errorf("months: got %d, want 6", trend.Months)
	}
	if len(trend.Points) != 6 {
		t.Errorf("len(points): got %d, want 6", len(trend.Points))
	}
}

func TestReportService_GetTrend_ZeroMonths(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	_, err := svc.GetTrend(context.Background(), 1, 0)
	if !errors.Is(err, reports.ErrInvalidMonths) {
		t.Errorf("expected ErrInvalidMonths, got %v", err)
	}
}

func TestReportService_GetTrend_TooManyMonths(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	_, err := svc.GetTrend(context.Background(), 1, 25) // max is 24
	if !errors.Is(err, reports.ErrInvalidMonths) {
		t.Errorf("expected ErrInvalidMonths, got %v", err)
	}
}

func TestReportService_GetTrend_BoundaryValues(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	// Minimum valid months = 1
	if _, err := svc.GetTrend(context.Background(), 1, 1); err != nil {
		t.Errorf("months=1 should be valid, got: %v", err)
	}
	// Maximum valid months = 24
	if _, err := svc.GetTrend(context.Background(), 1, 24); err != nil {
		t.Errorf("months=24 should be valid, got: %v", err)
	}
}

// ── GetExportRows tests ───────────────────────────────────────────────────────

func TestReportService_GetExportRows_Success(t *testing.T) {
	t.Parallel()
	year, month := pastYearMonth()
	repo := &mockReportRepo{
		getExportRowsFn: func(_ context.Context, _ uint, y, m int) ([]reports.ExportRow, error) {
			return []reports.ExportRow{
				{Description: "Swiggy", Amount: 250, Type: "expense"},
			}, nil
		},
	}
	svc := reports.NewService(repo)

	rows, err := svc.GetExportRows(context.Background(), 1, year, month)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("len(rows): got %d, want 1", len(rows))
	}
}

func TestReportService_GetExportRows_InvalidYear(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	_, err := svc.GetExportRows(context.Background(), 1, 1850, 6)
	if !errors.Is(err, reports.ErrInvalidYear) {
		t.Errorf("expected ErrInvalidYear, got %v", err)
	}
}

func TestReportService_GetExportRows_FutureMonth(t *testing.T) {
	t.Parallel()
	svc := reports.NewService(&mockReportRepo{})

	futureYear := time.Now().UTC().Year() + 2
	_, err := svc.GetExportRows(context.Background(), 1, futureYear, 3)
	if !errors.Is(err, reports.ErrFutureMonth) {
		t.Errorf("expected ErrFutureMonth, got %v", err)
	}
}

// ── Table-driven validation test ──────────────────────────────────────────────

func TestReportService_GetMonthlySummary_ValidationTable(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	year, month := pastYearMonth()

	tests := []struct {
		name    string
		year    int
		month   int
		wantErr error
	}{
		{"valid past month",          year,                      month,             nil},
		{"year too low",              1999,                      6,                 reports.ErrInvalidYear},
		{"year too high",             2101,                      6,                 reports.ErrInvalidYear},
		{"month zero",                2024,                      0,                 reports.ErrInvalidMonth},
		{"month thirteen",            2024,                      13,                reports.ErrInvalidMonth},
		{"future year",               now.Year() + 1,           1,                 reports.ErrFutureMonth},
		{"boundary year 2000",        2000,                      1,                 nil},
		{"boundary year 2100",        2100,                      1,                 reports.ErrFutureMonth},
	}

	svc := reports.NewService(&mockReportRepo{})

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := svc.GetMonthlySummary(context.Background(), 1, tc.year, tc.month)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("got %v, want %v", err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}
