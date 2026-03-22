package rules

import "time"

// Rule is the GORM model that mirrors the categorization_rules table.
//
// A rule says: "for this user, if the transaction description contains
// Keyword, assign CategoryID."
// Priority allows users to rank rules — higher number wins. Ties are
// broken by ID (older rule takes precedence).
type Rule struct {
	ID         uint      `gorm:"primaryKey"        json:"id"`
	UserID     uint      `gorm:"not null;index"    json:"user_id"`
	CategoryID uint      `gorm:"not null"          json:"category_id"`
	Keyword    string    `gorm:"size:100;not null" json:"keyword"`
	Priority   int       `gorm:"not null;default:0" json:"priority"`
	CreatedAt  time.Time `                         json:"created_at"`
	UpdatedAt  time.Time `                         json:"updated_at"`
}

// TableName overrides the default GORM table name.
func (Rule) TableName() string { return "categorization_rules" }

// ─────────────────────────────────────────────────────────────────────────────
// Request / Response types
// ─────────────────────────────────────────────────────────────────────────────

// CreateRuleRequest carries the fields required to create a new rule.
type CreateRuleRequest struct {
	// CategoryID is the category to assign when the keyword matches.
	CategoryID uint `json:"category_id" binding:"required"`
	// Keyword is the case-insensitive substring to match against the transaction description.
	Keyword string `json:"keyword" binding:"required,max=100"`
	// Priority (optional, default 0). Higher number wins when multiple rules match.
	Priority int `json:"priority"`
}

// RuleResponse is the JSON shape returned to the client.
type RuleResponse struct {
	ID         uint      `json:"id"`
	CategoryID uint      `json:"category_id"`
	Keyword    string    `json:"keyword"`
	Priority   int       `json:"priority"`
	CreatedAt  time.Time `json:"created_at"`
}

// RuleListResponse is the JSON shape returned for a list of rules.
type RuleListResponse struct {
	Rules []RuleResponse `json:"rules"`
	Total int            `json:"total"`
}

// toRuleResponse converts a Rule model to its API response shape.
func toRuleResponse(r Rule) RuleResponse {
	return RuleResponse{
		ID:         r.ID,
		CategoryID: r.CategoryID,
		Keyword:    r.Keyword,
		Priority:   r.Priority,
		CreatedAt:  r.CreatedAt,
	}
}

// toRuleResponseList converts a slice of Rule models to response shapes.
func toRuleResponseList(rs []Rule) []RuleResponse {
	out := make([]RuleResponse, len(rs))
	for i, r := range rs {
		out[i] = toRuleResponse(r)
	}
	return out
}
