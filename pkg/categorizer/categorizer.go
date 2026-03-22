// Package categorizer provides a pure keyword-based rules engine that
// automatically assigns a category to a transaction description.
//
// Design:
//   - Rules are loaded from the database per user (passed in by the caller).
//   - Matching is a case-insensitive substring search — no regex, no ML.
//   - When multiple rules match, the one with the highest Priority wins.
//     Ties are broken by the rule's ID (lower ID = created first = higher priority).
//   - If no rule matches, the function returns (0, false) and the caller
//     leaves category_id as NULL.
//
// This package is intentionally dependency-free (no DB, no HTTP) so it can
// be unit-tested without any infrastructure.
package categorizer

import (
	"strings"
)

// Rule is the data the engine needs to perform a match.
// The caller loads all rules for the user from the DB and passes them here.
type Rule struct {
	ID         uint   // primary key (used for tie-breaking)
	Keyword    string // substring to search for (case-insensitive)
	CategoryID uint   // category to assign when matched
	Priority   int    // higher wins; ties broken by lowest ID
}

// Categorize takes a transaction description and a list of rules for the user,
// and returns the CategoryID of the best-matching rule.
//
// Returns (categoryID, true) when a rule matches, or (0, false) when none do.
//
// The function is a pure function — no side effects, no database access.
func Categorize(description string, rules []Rule) (uint, bool) {
	if description == "" || len(rules) == 0 {
		return 0, false
	}

	lower := strings.ToLower(description)

	var best *Rule
	for i := range rules {
		r := &rules[i]
		if !strings.Contains(lower, strings.ToLower(r.Keyword)) {
			continue
		}
		// First match, or better priority, or same priority but lower ID.
		if best == nil ||
			r.Priority > best.Priority ||
			(r.Priority == best.Priority && r.ID < best.ID) {
			best = r
		}
	}

	if best == nil {
		return 0, false
	}
	return best.CategoryID, true
}
