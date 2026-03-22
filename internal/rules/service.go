package rules

import (
	"context"
	"errors"

	"github.com/nilabh/arthaledger/pkg/categorizer"
	"gorm.io/gorm"
)

// Sentinel errors returned by the service layer.
var (
	// ErrRuleNotFound is returned when the rule does not exist or belongs to
	// a different user (404 — prevents ID enumeration).
	ErrRuleNotFound = errors.New("rule not found")

	// ErrDuplicateKeyword is returned when the user already has a rule with
	// the same keyword (the DB unique index on (user_id, lower(keyword)) fires).
	ErrDuplicateKeyword = errors.New("a rule with this keyword already exists")
)

// Service defines the business-logic operations for categorization rules.
type Service interface {
	// Create validates and persists a new rule for the given user.
	Create(ctx context.Context, userID uint, req CreateRuleRequest) (*RuleResponse, error)

	// List returns all rules for the user.
	List(ctx context.Context, userID uint) (*RuleListResponse, error)

	// Delete permanently removes a user-owned rule.
	Delete(ctx context.Context, userID uint, ruleID uint) error

	// CategorizerRules returns the rules in the thin shape expected by the
	// categorizer package — used by the transaction service on every create.
	CategorizerRules(ctx context.Context, userID uint) ([]categorizer.Rule, error)
}

// service is the concrete implementation of Service.
type service struct {
	repo Repository
}

// NewService creates a Service backed by the given Repository.
func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// Create validates the request and inserts a new rule.
func (s *service) Create(ctx context.Context, userID uint, req CreateRuleRequest) (*RuleResponse, error) {
	rule := &Rule{
		UserID:     userID,
		CategoryID: req.CategoryID,
		Keyword:    req.Keyword,
		Priority:   req.Priority,
	}
	if err := s.repo.Create(ctx, rule); err != nil {
		// The unique index on (user_id, lower(keyword)) produces a unique-violation
		// error. Wrap it as a friendlier sentinel so the handler can return 409.
		if isUniqueViolation(err) {
			return nil, ErrDuplicateKeyword
		}
		return nil, err
	}
	resp := toRuleResponse(*rule)
	return &resp, nil
}

// List returns all rules for the user ordered by priority DESC, id ASC.
func (s *service) List(ctx context.Context, userID uint) (*RuleListResponse, error) {
	rs, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &RuleListResponse{
		Rules: toRuleResponseList(rs),
		Total: len(rs),
	}, nil
}

// Delete verifies ownership then removes the rule.
func (s *service) Delete(ctx context.Context, userID uint, ruleID uint) error {
	// FindByIDAndUserID returns gorm.ErrRecordNotFound for non-existent or wrong-owner IDs.
	if _, err := s.repo.FindByIDAndUserID(ctx, ruleID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRuleNotFound
		}
		return err
	}
	return s.repo.Delete(ctx, ruleID)
}

// CategorizerRules delegates to the repository's lean projection.
func (s *service) CategorizerRules(ctx context.Context, userID uint) ([]categorizer.Rule, error) {
	return s.repo.ListAsCategorizer(ctx, userID)
}

// isUniqueViolation detects a PostgreSQL unique-constraint violation.
// We inspect the error string to avoid importing the pq/pgconn driver directly.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// PostgreSQL error code 23505 = unique_violation
	return contains(msg, "23505") || contains(msg, "unique constraint") || contains(msg, "duplicate key")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
