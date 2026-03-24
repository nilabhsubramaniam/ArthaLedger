package alerts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// ── Sentinel errors ────────────────────────────────────────────────────────────

var (
	// ErrAlertNotFound is returned when the alert does not exist or belongs
	// to a different user. A single error prevents ID-enumeration attacks.
	ErrAlertNotFound = errors.New("alert not found")
)

// ── Service interface ──────────────────────────────────────────────────────────

// Service is the business-logic contract for the alerts domain.
//
// It also satisfies the budgets.AlertCreator interface via CreateBudgetAlert,
// which is the bridge that lets the budgets service fire alerts without
// importing this package (Go structural typing handles the match in main.go).
type Service interface {
	// List returns all alerts for the user with total and unread counts.
	List(ctx context.Context, userID uint) (*AlertListResponse, error)

	// MarkRead marks a single alert as read.
	MarkRead(ctx context.Context, id, userID uint) error

	// MarkAllRead marks every unread alert as read for the user.
	MarkAllRead(ctx context.Context, userID uint) error

	// Delete hard-deletes a single alert.
	Delete(ctx context.Context, id, userID uint) error

	// CreateBudgetAlert creates an in-app alert tied to a budget, but only
	// if no identical unread alert already exists (deduplication).
	// This method satisfies the budgets.AlertCreator interface.
	CreateBudgetAlert(ctx context.Context, userID uint, budgetID uint, alertType, title, message string) error
}

// ── Concrete implementation ────────────────────────────────────────────────────

type service struct {
	repo Repository
}

// NewService constructs the alerts service.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// ── Method implementations ─────────────────────────────────────────────────────

// List returns all alerts for the user, newest first, with total and unread count.
func (s *service) List(ctx context.Context, userID uint) (*AlertListResponse, error) {
	rows, total, unread, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing alerts: %w", err)
	}

	responses := make([]AlertResponse, len(rows))
	for i, a := range rows {
		responses[i] = toAlertResponse(a)
	}

	return &AlertListResponse{
		Alerts:      responses,
		Total:       total,
		UnreadCount: unread,
	}, nil
}

// MarkRead marks a single alert as read.
func (s *service) MarkRead(ctx context.Context, id, userID uint) error {
	if err := s.repo.MarkRead(ctx, id, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAlertNotFound
		}
		return fmt.Errorf("marking alert read: %w", err)
	}
	return nil
}

// MarkAllRead marks every unread alert as read for the user.
func (s *service) MarkAllRead(ctx context.Context, userID uint) error {
	if err := s.repo.MarkAllRead(ctx, userID); err != nil {
		return fmt.Errorf("marking all alerts read: %w", err)
	}
	slog.Info("All alerts marked read", "user_id", userID)
	return nil
}

// Delete hard-deletes a single alert. A deleted alert cannot be recovered.
func (s *service) Delete(ctx context.Context, id, userID uint) error {
	if err := s.repo.Delete(ctx, id, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAlertNotFound
		}
		return fmt.Errorf("deleting alert: %w", err)
	}
	return nil
}

// CreateBudgetAlert creates an alert tied to a specific budget.
//
// Deduplication: if an unread alert of the same type already exists for this
// budget, the method returns nil without inserting a duplicate. This prevents
// the alert list from flooding when a user views their budgets repeatedly.
func (s *service) CreateBudgetAlert(
	ctx context.Context,
	userID uint,
	budgetID uint,
	alertType, title, message string,
) error {
	// Deduplication check: skip if an identical unread alert already exists.
	exists, err := s.repo.ExistsUnreadForBudget(ctx, userID, budgetID, AlertType(alertType))
	if err != nil {
		return fmt.Errorf("checking existing budget alert: %w", err)
	}
	if exists {
		// Already notified — no duplicate needed.
		return nil
	}

	alert := &Alert{
		UserID:   userID,
		BudgetID: &budgetID,
		Type:     AlertType(alertType),
		Title:    title,
		Message:  message,
		IsRead:   false,
	}

	if err := s.repo.Create(ctx, alert); err != nil {
		return fmt.Errorf("creating budget alert: %w", err)
	}

	slog.Info("Budget alert created",
		"user_id", userID,
		"budget_id", budgetID,
		"type", alertType,
	)
	return nil
}
