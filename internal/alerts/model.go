package alerts

import "time"

// ── AlertType enum ─────────────────────────────────────────────────────────────

// AlertType classifies the reason an alert was generated.
// Stored as VARCHAR(50) in the alerts table.
type AlertType string

const (
	// AlertBudgetWarning is raised when spending reaches 80 % of a budget limit.
	AlertBudgetWarning AlertType = "budget_warning"

	// AlertBudgetExceeded is raised when spending equals or exceeds a budget limit.
	AlertBudgetExceeded AlertType = "budget_exceeded"

	// AlertUnusualSpending is raised when a single transaction is significantly
	// higher than the user's 30-day average for that category (future expansion).
	AlertUnusualSpending AlertType = "unusual_spending"
)

// ── Database model ─────────────────────────────────────────────────────────────

// Alert is the GORM model mapping to the `alerts` table (created in 000001,
// extended with budget_id in 000004).
//
// Alerts are append-only: they are never updated except to set is_read = true.
// Deletion is hard (no soft-delete) — a dismissed alert is gone for good.
type Alert struct {
	ID     uint `gorm:"primaryKey"                      json:"id"`
	UserID uint `gorm:"not null;index"                  json:"user_id"`

	// BudgetID links the alert to the budget that triggered it (nullable).
	// Set for budget_warning and budget_exceeded alerts; nil for generic alerts.
	BudgetID *uint `gorm:"index"                         json:"budget_id,omitempty"`

	Type    AlertType `gorm:"type:varchar(50);not null"       json:"type"`
	Title   string    `gorm:"type:varchar(255);not null"      json:"title"`
	Message string    `gorm:"type:text;not null"             json:"message"`
	IsRead  bool      `gorm:"not null;default:false"          json:"is_read"`

	// No UpdatedAt — alerts are immutable apart from the is_read flag.
	// No DeletedAt — deletion is hard to keep the table lean.
	CreatedAt time.Time `json:"created_at"`
}

// TableName overrides GORM table-name inference.
func (Alert) TableName() string { return "alerts" }

// ── Response types ─────────────────────────────────────────────────────────────

// AlertResponse is the JSON shape returned to the client for a single alert.
type AlertResponse struct {
	ID        uint      `json:"id"`
	UserID    uint      `json:"user_id"`
	BudgetID  *uint     `json:"budget_id,omitempty"`
	Type      AlertType `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// AlertListResponse wraps a list of alerts with metadata.
type AlertListResponse struct {
	Alerts      []AlertResponse `json:"alerts"`
	Total       int64           `json:"total"`
	UnreadCount int64           `json:"unread_count"`
}

// ── Helper converters ──────────────────────────────────────────────────────────

// toAlertResponse converts an Alert model to its API response shape.
func toAlertResponse(a Alert) AlertResponse {
	return AlertResponse{
		ID:        a.ID,
		UserID:    a.UserID,
		BudgetID:  a.BudgetID,
		Type:      a.Type,
		Title:     a.Title,
		Message:   a.Message,
		IsRead:    a.IsRead,
		CreatedAt: a.CreatedAt,
	}
}
