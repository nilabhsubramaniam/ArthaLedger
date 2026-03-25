package categorizer_test

// Pure unit tests for the categorizer package.
// No DB, no network — the categorizer is a pure function and can be tested
// in complete isolation from all infrastructure.

import (
	"testing"

	"github.com/nilabh/arthaledger/pkg/categorizer"
)

// ── Helper ────────────────────────────────────────────────────────────────────

// rules is a shared test fixture used across multiple test cases.
var testRules = []categorizer.Rule{
	{ID: 1, Keyword: "swiggy",    CategoryID: 10, Priority: 5},
	{ID: 2, Keyword: "zomato",    CategoryID: 10, Priority: 5},
	{ID: 3, Keyword: "uber",      CategoryID: 20, Priority: 3},
	{ID: 4, Keyword: "salary",    CategoryID: 30, Priority: 10},
	{ID: 5, Keyword: "SALARY",    CategoryID: 31, Priority: 9},  // lower priority — should lose to ID 4
	{ID: 6, Keyword: "amazon",    CategoryID: 40, Priority: 5},
	{ID: 7, Keyword: "flipkart",  CategoryID: 40, Priority: 5},
}

// ── Table-driven tests ─────────────────────────────────────────────────────────

func TestCategorize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		description     string
		rules           []categorizer.Rule
		wantCategoryID  uint
		wantMatched     bool
	}{
		{
			name:           "exact keyword match",
			description:    "Payment to Swiggy for dinner",
			rules:          testRules,
			wantCategoryID: 10,
			wantMatched:    true,
		},
		{
			name:           "case-insensitive match — uppercase description",
			description:    "ZOMATO ORDER #12345",
			rules:          testRules,
			wantCategoryID: 10,
			wantMatched:    true,
		},
		{
			name:           "higher priority wins when multiple keywords match",
			// "salary" has priority 10, "SALARY" has priority 9 → rule ID 4 wins
			description:    "SALARY CREDIT HDFC",
			rules:          testRules,
			wantCategoryID: 30,
			wantMatched:    true,
		},
		{
			name:           "lower priority rule does not override higher priority",
			description:    "salary march 2026",
			rules:          testRules,
			wantCategoryID: 30,
			wantMatched:    true,
		},
		{
			name:           "no rule matches",
			description:    "ATM withdrawal",
			rules:          testRules,
			wantCategoryID: 0,
			wantMatched:    false,
		},
		{
			name:           "empty description returns no match",
			description:    "",
			rules:          testRules,
			wantCategoryID: 0,
			wantMatched:    false,
		},
		{
			name:           "empty rule list returns no match",
			description:    "Swiggy food",
			rules:          []categorizer.Rule{},
			wantCategoryID: 0,
			wantMatched:    false,
		},
		{
			name:           "nil rule list returns no match",
			description:    "Swiggy food",
			rules:          nil,
			wantCategoryID: 0,
			wantMatched:    false,
		},
		{
			name:           "keyword is a substring of the description",
			description:    "refund from amazon prime",
			rules:          testRules,
			wantCategoryID: 40,
			wantMatched:    true,
		},
		{
			name: "tie in priority broken by lower ID",
			// amazon (ID 6) and flipkart (ID 7) both priority 5 →
			// description contains only amazon → amazon wins
			description:    "Amazon order shipped",
			rules:          testRules,
			wantCategoryID: 40,
			wantMatched:    true,
		},
		{
			name: "single rule — match",
			description:    "uber ride home",
			rules:          []categorizer.Rule{{ID: 1, Keyword: "uber", CategoryID: 99, Priority: 0}},
			wantCategoryID: 99,
			wantMatched:    true,
		},
	}

	for _, tc := range tests {
		tc := tc // capture loop variable for parallel sub-tests
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotID, gotMatched := categorizer.Categorize(tc.description, tc.rules)
			if gotMatched != tc.wantMatched {
				t.Errorf("matched: got %v, want %v", gotMatched, tc.wantMatched)
			}
			if gotID != tc.wantCategoryID {
				t.Errorf("categoryID: got %d, want %d", gotID, tc.wantCategoryID)
			}
		})
	}
}

func TestCategorize_TieBrokenByLowerID(t *testing.T) {
	t.Parallel()

	// Both rules match the description and share the same priority.
	// The rule with the lower ID should win.
	rules := []categorizer.Rule{
		{ID: 10, Keyword: "food", CategoryID: 100, Priority: 5},
		{ID: 5,  Keyword: "food", CategoryID: 200, Priority: 5},
	}

	categoryID, matched := categorizer.Categorize("food delivery", rules)
	if !matched {
		t.Fatal("expected a match, got none")
	}
	if categoryID != 200 {
		t.Errorf("expected category 200 (lower ID wins), got %d", categoryID)
	}
}
