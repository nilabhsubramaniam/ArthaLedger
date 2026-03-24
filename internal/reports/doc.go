// Package reports generates read-only financial analytics derived from the
// transactions and accounts tables.
//
// Three report types are supported:
//   - Monthly summary  — income, expenses, and net savings for one calendar month,
//     broken down by category with percentage shares.
//   - Spending trends  — income vs. expense vs. net for the last N months.
//   - CSV export       — all transactions for a given month as a downloadable file.
//
// Reports are read-only: this package never writes to the database.
//
// Layer responsibilities:
//
//	model.go      — response / DTO types (no GORM model — no own table)
//	repository.go — complex SQL aggregation queries (joins transactions + categories)
//	service.go    — parameter validation, date arithmetic, response assembly
//	handler.go    — HTTP handlers with Swagger annotations and CSV streaming
package reports
