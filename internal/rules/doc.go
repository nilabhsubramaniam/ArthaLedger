// Package rules manages the user-defined categorization rules that power the
// auto-categorization engine. Each rule maps a keyword to a category — when a
// transaction is created without an explicit category_id, the engine scans the
// user's rules and assigns the best match automatically.
package rules
