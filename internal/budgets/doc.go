// Package budgets manages monthly (and other period) spending limits per category.
//
// A budget ties a spending limit to a category and a time period. The service
// computes how much has already been spent against each budget dynamically by
// querying the transactions table — no denormalised "spent" column is stored.
//
// Layer responsibilities:
//
//	model.go      — GORM struct, type enums, request/response types
//	repository.go — database queries + live spent-amount computation
//	service.go    — business rules (ownership, date validation, alert triggering)
//	handler.go    — HTTP handlers wired to Gin routes with Swagger annotations
package budgets
