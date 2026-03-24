package budgets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"gorm.io/gorm"
)

// ── Sentinel errors ────────────────────────────────────────────────────────────

var (
	// ErrBudgetNotFound is returned when the budget does not exist or belongs
	// to a different user. A single error prevents ID-enumeration attacks.
	ErrBudgetNotFound = errors.New("budget not found")

	// ErrInvalidPeriod is returned when the period field is not one of the four
	// allowed values: daily | weekly | monthly | yearly.
	ErrInvalidPeriod = errors.New("invalid budget period, must be daily | weekly | monthly | yearly")

	// ErrInvalidDate is returned when a date string cannot be parsed as YYYY-MM-DD.
	ErrInvalidDate = errors.New("invalid date format, expected YYYY-MM-DD")

	// ErrEndBeforeStart is returned when end_date is not strictly after start_date.
	ErrEndBeforeStart = errors.New("end_date must be after start_date")

	// ErrNoUpdates is returned when a PUT body contains no recognisable fields.
	ErrNoUpdates = errors.New("no updatable fields provided")
)

// ── AlertCreator interface ─────────────────────────────────────────────────────

// AlertCreator is the thin interface that allows the budget service to create
// in-app alerts without importing the alerts package directly.
//
// This one-method interface breaks the potential import cycle:
//
//	budgets → alerts  (via this interface satisfied by alerts.Service)
//
// The concrete implementation is alerts.Service; it is injected at startup in
// main.go after both services are constructed.
type AlertCreator interface {
	// CreateBudgetAlert persists an alert tied to a specific budget.
	// alertType should be one of: "budget_warning", "budget_exceeded".
	CreateBudgetAlert(ctx context.Context, userID uint, budgetID uint, alertType, title, message string) error
}

// ── Service interface ──────────────────────────────────────────────────────────

// Service is the business-logic contract for the budgets domain.
// Handlers depend only on this interface — the concrete struct is never exposed.
type Service interface {
	Create(ctx context.Context, userID uint, req CreateBudgetRequest) (*BudgetResponse, error)
	List(ctx context.Context, userID uint) (*BudgetListResponse, error)
	GetByID(ctx context.Context, id, userID uint) (*BudgetResponse, error)
	Update(ctx context.Context, id, userID uint, req UpdateBudgetRequest) (*BudgetResponse, error)
	Delete(ctx context.Context, id, userID uint) error
}

// ── Concrete implementation ────────────────────────────────────────────────────

type service struct {
	repo         Repository
	alertCreator AlertCreator // may be nil (tests / early boot)
}

// NewService constructs the budgets service.
// alertCreator may be nil — if so, alert creation is silently skipped.
func NewService(repo Repository, alertCreator AlertCreator) Service {
	return &service{repo: repo, alertCreator: alertCreator}
}

// ── Public method implementations ─────────────────────────────────────────────

// Create validates the request and inserts a new budget row.
//
// Business rules:
//   - Period must be one of: daily | weekly | monthly | yearly.
//   - start_date must be a valid YYYY-MM-DD string.
//   - end_date, when provided, must be after start_date.
//   - Ownership is derived from the JWT; the caller never supplies user_id.
func (s *service) Create(ctx context.Context, userID uint, req CreateBudgetRequest) (*BudgetResponse, error) {
	if !req.Period.IsValid() {
		return nil, ErrInvalidPeriod
	}

	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return nil, ErrInvalidDate
	}

	var endDate *time.Time
	if req.EndDate != "" {
		parsed, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			return nil, ErrInvalidDate
		}
		if !parsed.After(startDate) {
			return nil, ErrEndBeforeStart
		}
		endDate = &parsed
	}

	budget := &Budget{
		UserID:     userID,
		CategoryID: req.CategoryID,
		Name:       req.Name,
		Amount:     req.Amount,
		Period:     req.Period,
		StartDate:  startDate,
		EndDate:    endDate,
		IsActive:   true,
	}

	if err := s.repo.Create(ctx, budget); err != nil {
		return nil, fmt.Errorf("creating budget: %w", err)
	}

	slog.Info("Budget created", "user_id", userID, "budget_id", budget.ID, "name", budget.Name)
	return s.buildResponse(ctx, budget)
}

// List returns all budgets for the user, each enriched with live spent data.
func (s *service) List(ctx context.Context, userID uint) (*BudgetListResponse, error) {
	budgets, total, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing budgets: %w", err)
	}

	responses := make([]BudgetResponse, 0, len(budgets))
	for i := range budgets {
		resp, err := s.buildResponse(ctx, &budgets[i])
		if err != nil {
			return nil, err
		}
		responses = append(responses, *resp)
	}

	return &BudgetListResponse{Budgets: responses, Total: total}, nil
}

// GetByID returns a single budget with live spent/remaining values.
func (s *service) GetByID(ctx context.Context, id, userID uint) (*BudgetResponse, error) {
	budget, err := s.repo.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBudgetNotFound
		}
		return nil, fmt.Errorf("fetching budget: %w", err)
	}
	return s.buildResponse(ctx, budget)
}

// Update applies partial field changes to an existing budget.
//
// Business rules:
//   - Period, if provided, must be valid.
//   - end_date, if provided, must be parseable as YYYY-MM-DD.
//   - At least one field must be present.
func (s *service) Update(ctx context.Context, id, userID uint, req UpdateBudgetRequest) (*BudgetResponse, error) {
	updates := make(map[string]interface{})

	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Amount > 0 {
		updates["amount"] = req.Amount
	}
	if req.Period != "" {
		if !req.Period.IsValid() {
			return nil, ErrInvalidPeriod
		}
		updates["period"] = req.Period
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if req.EndDate != "" {
		parsed, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			return nil, ErrInvalidDate
		}
		updates["end_date"] = parsed
	}

	if len(updates) == 0 {
		return nil, ErrNoUpdates
	}

	if err := s.repo.Update(ctx, id, userID, updates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBudgetNotFound
		}
		return nil, fmt.Errorf("updating budget: %w", err)
	}

	return s.GetByID(ctx, id, userID)
}

// Delete soft-deletes the budget.
func (s *service) Delete(ctx context.Context, id, userID uint) error {
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBudgetNotFound
		}
		return fmt.Errorf("deleting budget: %w", err)
	}
	slog.Info("Budget deleted", "user_id", userID, "budget_id", id)
	return nil
}

// ── Private helpers ────────────────────────────────────────────────────────────

// buildResponse enriches a Budget row with live spent / remaining / percent
// values, rounds amounts to two decimal places, and (as a best-effort side
// effect) fires budget alerts when thresholds are crossed.
func (s *service) buildResponse(ctx context.Context, b *Budget) (*BudgetResponse, error) {
	// Determine [from, to] for the currently active period window.
	from, to := currentPeriodWindow(b.StartDate, b.Period)

	spent, err := s.repo.GetSpentAmount(ctx, b.UserID, b.CategoryID, from, to)
	if err != nil {
		return nil, fmt.Errorf("computing spent for budget %d: %w", b.ID, err)
	}

	remaining := b.Amount - spent
	// PercentUsed is capped at 100 in the response; Remaining can still be negative.
	percent := math.Min((spent/b.Amount)*100, 100)

	resp := &BudgetResponse{
		ID:          b.ID,
		UserID:      b.UserID,
		CategoryID:  b.CategoryID,
		Name:        b.Name,
		Amount:      b.Amount,
		Period:      b.Period,
		StartDate:   b.StartDate,
		EndDate:     b.EndDate,
		IsActive:    b.IsActive,
		Spent:       math.Round(spent*100) / 100,
		Remaining:   math.Round(remaining*100) / 100,
		PercentUsed: math.Round(percent*100) / 100,
		CreatedAt:   b.CreatedAt,
		UpdatedAt:   b.UpdatedAt,
	}

	// Alert creation is non-fatal: a failure must not degrade the normal response.
	if b.IsActive && s.alertCreator != nil {
		s.maybeFireAlert(ctx, b, resp)
	}

	return resp, nil
}

// maybeFireAlert dispatches a budget_warning (≥80 %) or budget_exceeded (≥100 %)
// alert. Errors are logged at WARN level and swallowed so the caller always
// receives a complete BudgetResponse even if alert creation fails.
func (s *service) maybeFireAlert(ctx context.Context, b *Budget, resp *BudgetResponse) {
	var alertType, title, message string

	switch {
	case resp.Spent >= b.Amount:
		alertType = "budget_exceeded"
		title = fmt.Sprintf("Budget exceeded: %s", b.Name)
		message = fmt.Sprintf(
			`You have exceeded your budget "%s" by %.2f (limit: %.2f, spent: %.2f).`,
			b.Name, resp.Spent-b.Amount, b.Amount, resp.Spent,
		)
	case resp.PercentUsed >= 80:
		alertType = "budget_warning"
		title = fmt.Sprintf("Budget warning: %s (%.1f%% used)", b.Name, resp.PercentUsed)
		message = fmt.Sprintf(
			`You have used %.1f%% of your budget "%s" (limit: %.2f, spent: %.2f).`,
			resp.PercentUsed, b.Name, b.Amount, resp.Spent,
		)
	default:
		return // Below 80 % — no alert needed.
	}

	if err := s.alertCreator.CreateBudgetAlert(ctx, b.UserID, b.ID, alertType, title, message); err != nil {
		slog.Warn("Failed to create budget alert", "budget_id", b.ID, "type", alertType, "error", err)
	}
}

// currentPeriodWindow returns the [from, to] inclusive date range for the
// currently active budget window based on the period type and today's date.
//
// All returned times are in UTC, with from at 00:00:00 and to at 23:59:59
// so that date comparisons with PostgreSQL DATE columns work correctly.
//
// Examples (today = 2026-03-24):
//
//	daily   → 2026-03-24 … 2026-03-24
//	weekly  → 2026-03-23 (Mon) … 2026-03-29 (Sun)
//	monthly → 2026-03-01 … 2026-03-31
//	yearly  → 2026-01-01 … 2026-12-31
func currentPeriodWindow(startDate time.Time, period BudgetPeriod) (time.Time, time.Time) {
	now := time.Now().UTC()

	switch period {
	case PeriodDaily:
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		to := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
		return from, to

	case PeriodWeekly:
		// Anchor the window to the most recent Monday (ISO week).
		weekday := int(now.Weekday()) // Sunday = 0
		if weekday == 0 {
			weekday = 7 // treat Sunday as 7 so Monday = 1
		}
		monday := now.AddDate(0, 0, -(weekday - 1))
		from := time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC)
		to := from.AddDate(0, 0, 6)
		to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, time.UTC)
		return from, to

	case PeriodMonthly:
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		// First day of next month minus one day = last day of this month.
		lastDay := from.AddDate(0, 1, -1)
		to := time.Date(lastDay.Year(), lastDay.Month(), lastDay.Day(), 23, 59, 59, 0, time.UTC)
		return from, to

	case PeriodYearly:
		from := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(now.Year(), 12, 31, 23, 59, 59, 0, time.UTC)
		return from, to

	default:
		// Fallback: treat the entire span from start_date to today as the window.
		from := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
		to := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
		return from, to
	}
}
