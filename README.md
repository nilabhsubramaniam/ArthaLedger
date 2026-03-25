# ArthaLedger — Personal Finance Tracker API

A production-quality REST API backend for personal finance management, built with Go. It tracks income and expenses across multiple financial accounts, automatically categorises transactions using a user-defined keyword-rule engine, enforces spending budgets with live utilisation calculations, fires smart in-app alerts when budget thresholds are crossed, and generates monthly financial summaries, multi-month trend reports, and CSV exports.

---

## What This Project Does

Most personal finance apps require either a paid subscription or a mobile app. ArthaLedger is a self-hosted alternative you run yourself — a single binary backed by PostgreSQL and (optionally) Redis.

**Core capabilities:**

- **Multi-account tracking** — Bank accounts, cash wallets, credit cards, and investment portfolios each have their own balance. Every transaction updates the corresponding balance atomically; no balance can ever drift out of sync with its transactions.

- **Automatic transaction categorisation** — Users create keyword rules (e.g. keyword `"swiggy"` → category `Food & Dining`). When a new transaction is created without an explicit category, the rules engine matches the description against all rules and assigns the highest-priority matching category automatically. 16 system categories are seeded out of the box (Salary, Rent, Groceries, Transport, etc.).

- **Budgets with live spend tracking** — A budget defines a spending limit for a period (daily / weekly / monthly / yearly), optionally scoped to one category. Every time a budget is fetched, its current-period spend is computed live from the transactions table so there is never a stale cached value. Remaining balance and percentage used are returned alongside every budget response.

- **Smart alerts** — When a budget reaches 80 % utilisation a `budget_warning` alert is created. When it reaches or exceeds 100 % a `budget_exceeded` alert is created. Alerts are deduplicated — if an identical unread alert already exists, no duplicate is inserted. Users can mark alerts read individually or all at once.

- **Financial reports** — Monthly income/expense summaries with per-category expense breakdowns, multi-month income/expense/net trend data, and raw transaction CSV export for offline analysis.

- **Security by design** — JWT HS256 access tokens with per-token JTI blacklisting on logout (Redis), bcrypt cost-12 password hashing, row-level ownership enforced at the SQL layer on every query, and a single consistent error message for any "not found or wrong owner" case to prevent ID enumeration attacks.

---

## Tech Stack

| Layer | Technology | Why |
|---|---|---|
| Language | Go 1.23 | Compiled, strongly typed, excellent concurrency primitives, first-class HTTP support |
| HTTP Framework | Gin v1.9 | Minimal overhead, excellent middleware system, built-in request validation via struct tags |
| ORM | GORM + pgx driver | Type-safe queries with automatic migrations support; pgx gives native PostgreSQL types |
| Database | PostgreSQL 18 | ACID transactions, row-level locks, native `numeric` type for money (avoids float rounding) |
| Cache / Token store | Redis 7 | Stores refresh tokens and blacklisted JTIs — O(1) revocation check on every request |
| Configuration | Viper (reads `.env`) | Zero hardcoded values; same binary runs in dev, staging, and production |
| Migrations | golang-migrate | Version-controlled, deterministic schema changes with up/down rollback |
| Auth | JWT HS256 (`golang-jwt/jwt/v5`) | Stateless access tokens; Redis handles the stateful revocation side |
| Logging | `log/slog` (stdlib) | Structured key-value logging; zero external dependency |
| Documentation | Swagger UI (`swaggo/swag`) | Auto-generated from Go annotation comments; always stays in sync with code |
| Testing | stdlib `testing` | No external test libraries; hand-written mock repositories keep tests fast and dependency-free |
| Containerization | Docker & Docker Compose | Reproducible development environment; production-ready two-stage Docker build |

---

## Project Structure

```
ArthaLedger/
├── cmd/server/
│   └── main.go                     # Entry point — wires all dependencies, registers routes, starts server
├── config/
│   └── config.go                   # Loads .env into a typed Config struct via Viper
├── internal/                       # Business logic — one sub-package per domain
│   ├── auth/
│   │   ├── model.go                # User DB model + request/response types
│   │   ├── repository.go           # PostgreSQL implementation (FindByEmail, FindByID, Create)
│   │   ├── service.go              # Register, Login (bcrypt+JWT), Refresh, Logout (Redis blacklist)
│   │   ├── handler.go              # Gin HTTP handlers with Swagger annotations
│   │   └── service_test.go         # 8 unit tests
│   ├── accounts/
│   │   ├── model.go                # Account DB model, AccountType enum, request/response types
│   │   ├── repository.go           # CRUD + UpdateBalance (called inside DB transactions)
│   │   ├── service.go              # Type validation, ownership guards, balance immutability rules
│   │   ├── handler.go              # Gin HTTP handlers with Swagger annotations
│   │   └── service_test.go         # 14 unit tests
│   ├── transactions/
│   │   ├── model.go                # Transaction DB model, TypeEnum, filter/pagination types
│   │   ├── repository.go           # CRUD, transfer-pair insert, filtering & pagination
│   │   ├── service.go              # Atomic balance update, transfer pair logic, auto-categorization
│   │   ├── handler.go              # Gin HTTP handlers with Swagger annotations
│   │   └── service_test.go         # 13 unit tests
│   ├── categories/
│   │   ├── model.go                # Category DB model, CategoryType enum, request/response types
│   │   ├── repository.go           # CRUD + list system+user combined
│   │   ├── service.go              # System-category protection, ownership enforcement
│   │   ├── handler.go              # Gin HTTP handlers with Swagger annotations
│   │   └── service_test.go         # 17 unit tests
│   ├── rules/
│   │   ├── model.go                # Rule DB model (keyword → category mapping), request/response types
│   │   ├── repository.go           # CRUD + ListAsCategorizer (thin projection for the rules engine)
│   │   ├── service.go              # Ownership, duplicate-keyword detection (PostgreSQL 23505)
│   │   ├── handler.go              # Gin HTTP handlers with Swagger annotations
│   │   └── service_test.go         # 8 unit tests
│   ├── budgets/
│   │   ├── model.go                # Budget DB model, BudgetPeriod enum, request/response types
│   │   ├── repository.go           # CRUD + GetSpentAmount (live aggregation from transactions)
│   │   ├── service.go              # Period validation, currentPeriodWindow, 80%/100% alert thresholds
│   │   ├── handler.go              # Gin HTTP handlers with Swagger annotations
│   │   └── service_test.go         # 17 unit tests
│   ├── alerts/
│   │   ├── model.go                # Alert DB model (AlertType enum), response types
│   │   ├── repository.go           # Create, List, MarkRead, MarkAllRead, Delete, ExistsUnreadForBudget
│   │   ├── service.go              # Satisfies budgets.AlertCreator; deduplicates unread alerts
│   │   ├── handler.go              # Gin HTTP handlers with Swagger annotations
│   │   └── service_test.go         # 10 unit tests
│   └── reports/
│       ├── model.go                # MonthlySummary, TrendPoint, TrendResponse, ExportRow types
│       ├── repository.go           # Read-only SQL: income/expense totals, category breakdown, trend
│       ├── service.go              # Year/month/future-month validation, trend window bounds
│       ├── handler.go              # Gin HTTP handlers (JSON + CSV streaming) with Swagger annotations
│       └── service_test.go         # 14 unit tests
├── pkg/                            # Shared, reusable packages
│   ├── categorizer/
│   │   ├── categorizer.go          # Pure keyword rules engine — Categorize(description, rules)
│   │   ├── doc.go                  # Package-level documentation
│   │   └── categorizer_test.go     # 11 unit tests (no DB, no HTTP)
│   ├── database/
│   │   ├── postgres.go             # Opens GORM connection with connection-pool config
│   │   └── redis.go                # Creates Redis client with timeouts
│   ├── middleware/
│   │   ├── auth.go                 # JWT verification middleware; injects userID into context
│   │   └── doc.go
│   └── mailer/
│       └── doc.go                  # Reserved for Phase 5 — SMTP email notifications
├── migrations/
│   ├── 000001_init_schema.up.sql   # Creates all tables; seeds 16 system categories
│   ├── 000001_init_schema.down.sql
│   ├── 000002_add_transfer_reference.up.sql   # Adds transfer_reference_id to transactions
│   ├── 000002_add_transfer_reference.down.sql
│   ├── 000003_add_categorization_rules.up.sql # Creates categorization_rules table
│   ├── 000003_add_categorization_rules.down.sql
│   ├── 000004_add_budget_alerts_phase3.up.sql  # Adds budget_id FK on alerts
│   └── 000004_add_budget_alerts_phase3.down.sql
├── docker-compose.yml              # PostgreSQL 18 + Redis 7 services
├── Dockerfile                      # Two-stage build: builder → alpine:3.19
├── Makefile                        # Developer shortcuts (see below)
├── .env.example                    # Config template — copy to .env and fill in values
└── go.mod / go.sum                 # Module definition and verified dependency hashes
```

### Architecture pattern

Every domain in `internal/` follows a strict three-layer pattern:

```
Handler  →  Service  →  Repository
```

- **Handler** — parses and validates the HTTP request, calls the service, writes the HTTP response. Contains no business logic.
- **Service** — owns all business rules, sentinel-error returns, and cross-domain orchestration. No SQL.
- **Repository** — owns all database queries. Every method accepts `context.Context` as its first argument and includes `userID` in every WHERE clause to enforce row-level ownership at the database layer.

All three layers depend only on **interfaces**, never on concrete structs. DB and Redis clients are injected via constructors — no globals anywhere. This design means every service is fully testable without a real database: swap the real repository implementation for a mock struct in tests, and the service runs in isolation.

**Interface boundary example — breaking a dependency cycle without a separate package:**
The `budgets` package needs to create alerts, but importing `internal/alerts` from `internal/budgets` would create a circular import. Instead, `budgets` defines a minimal `AlertCreator` interface:

```go
// internal/budgets/service.go
type AlertCreator interface {
    CreateBudgetAlert(ctx context.Context, userID, budgetID uuid.UUID, alertType string) error
}
```

`alerts.Service` satisfies this interface. `budgets.NewService` takes an `AlertCreator` — at wire-up time (`main.go`) the alert service is injected. Neither package ever imports the other.

---

## API

All routes are prefixed with `/api/v1`. All routes except `/auth/register` and `/auth/login` require an `Authorization: Bearer <token>` header.

| Group | Prefix | Key endpoints |
|---|---|---|
| Auth | `/api/v1/auth` | `POST /register`, `POST /login`, `POST /refresh`, `POST /logout` |
| Accounts | `/api/v1/accounts` | Full CRUD + `GET /:id/summary` |
| Transactions | `/api/v1/transactions` | Full CRUD with filters (type, category, date range, account) + pagination |
| Categories | `/api/v1/categories` | CRUD — system categories visible to all, user categories owned |
| Rules | `/api/v1/rules` | `POST`, `GET`, `DELETE /:id` — keyword → category mappings |
| Budgets | `/api/v1/budgets` | Full CRUD — each response includes live Spent / Remaining / PercentUsed |
| Alerts | `/api/v1/alerts` | `GET`, `PATCH /:id/read`, `PATCH /read-all`, `DELETE /:id` |
| Reports | `/api/v1/reports` | `GET /monthly`, `GET /trend`, `GET /export` (CSV streaming) |
| Health | `/health` | `GET` — liveness check |

Full interactive Swagger UI: `http://localhost:8080/swagger/index.html` (run `make swagger` first, then re-run after any annotation change).

All JSON responses use one of two shapes:
```json
{ "success": true,  "data":  <payload> }
{ "success": false, "error": "<message>" }
```

---

## Database Schema

Seven tables — created by running `make migrate-up`.

| Table | Purpose |
|---|---|
| `users` | Accounts with bcrypt-hashed passwords |
| `accounts` | Bank/cash/credit accounts per user |
| `categories` | System-wide (seeded) and user-defined categories |
| `transactions` | Income and expense records linked to an account |
| `categorization_rules` | Keyword → category mappings for auto-categorization |
| `budgets` | Monthly (and other period) spending limits per category |
| `alerts` | Budget warnings, breach notices, unusual spend flags |

16 system categories are seeded automatically (Salary, Food & Dining, Transport, Shopping, etc.).

### Migration history

| File | Change |
|---|---|
| `000001_init_schema` | All tables + seed categories |
| `000002_add_transfer_reference` | `transfer_reference_id` on transactions |
| `000003_add_categorization_rules` | `categorization_rules` table |
| `000004_add_budget_alerts_phase3` | `budget_id` FK on alerts (Phase 3) |

---

## Quick Start

### Prerequisites

- Go 1.23+
- PostgreSQL 18 running locally (or via Docker)
- Redis (optional in development — server starts without it with a warning)

### 1. Configure

```powershell
Copy-Item .env.example .env
# Edit .env with your DB credentials
```

### 2. Create the database

```powershell
$env:PGPASSWORD = "your-password"
& "C:\Program Files\PostgreSQL\18\bin\psql.exe" -U postgres -h localhost -c "CREATE DATABASE finance_tracker;"
```

### 3. Run migrations

```powershell
make migrate-up
```

### 4. Start the server

```powershell
make run
```

### 5. Verify

```powershell
Invoke-WebRequest -Uri http://localhost:8080/health -UseBasicParsing
# → { "success": true, "data": { "status": "ok" } }
```

### With Docker (PostgreSQL + Redis)

```powershell
make docker-up   # starts both services
make run         # starts the Go server
```

---

## Makefile Targets

```
make run          Run the server (go run ./cmd/server/main.go)
make build        Compile binary to bin/server
make test         Run all tests with verbose output
make test-cover   Run tests and open HTML coverage report
make migrate-up   Apply all pending migrations
make migrate-down Rollback the last migration
make swagger      Generate Swagger docs into docs/
make docker-up    Start PostgreSQL and Redis containers
make docker-down  Stop containers
make docker-logs  Stream container logs
make lint         Run golangci-lint
make tidy         Clean up go.mod and go.sum
make clean        Remove build artifacts (bin/)
```

---

## Configuration

Copy `.env.example` to `.env`. The application reads all settings from this file via Viper — nothing is hardcoded in source, so the same binary runs identically across development, staging, and production.

| Variable | Required | Description |
|---|---|---|
| `APP_ENV` | No (default `development`) | In `development` mode Redis failures are non-fatal (server starts anyway with a warning). In `production` a missing Redis connection is a hard startup error. |
| `SERVER_PORT` | No (default `8080`) | TCP port the HTTP server listens on. |
| `DB_HOST` | Yes | PostgreSQL hostname or IP. |
| `DB_PORT` | No (default `5432`) | PostgreSQL port. |
| `DB_NAME` | Yes | Database name. |
| `DB_USER` | Yes | Database user. |
| `DB_PASSWORD` | Yes | Database password. |
| `DB_SSL_MODE` | No (default `disable`) | Set to `require` or `verify-full` in production. |
| `REDIS_HOST` | Yes* | Redis hostname. *Skipped in `development` if absent. |
| `REDIS_PORT` | No (default `6379`) | Redis port. |
| `JWT_SECRET` | Yes | HS256 signing key. **Minimum 32 characters.** Use a cryptographic random value; store it in a secret manager in production, never in version control. |
| `JWT_ACCESS_EXPIRY` | No (default `15m`) | Access token lifetime. Short-lived by design — clients refresh using the refresh token. |
| `JWT_REFRESH_EXPIRY` | No (default `7d`) | Refresh token TTL stored in Redis. |

Transactions with no matching keyword rule fall back to the "Others" category automatically — no external AI service is required.

---

## Testing

The test suite has **102 unit tests** across every service layer and the categorizer utility, all using only the Go standard library (`testing` package). There are zero external testing dependencies — no mocking frameworks, no test databases, no Docker required.

```
make test          # run all tests
make test-cover    # run all tests and open HTML coverage report
```

### Test inventory

| Package | Tests | What is tested |
|---|---|---|
| `pkg/categorizer` | 11 | Pure `Categorize()`: exact match, case-insensitive, priority ordering, tie-break by lower ID, empty inputs |
| `internal/auth` | 8 | Register (success, duplicate email), Login (success, wrong password, inactive user), Refresh and Logout with nil Redis |
| `internal/accounts` | 14 | Full CRUD, invalid account type, `ErrHasTransactions` guard on delete, not-found, no-fields update |
| `internal/categories` | 17 | System vs user ownership, wrong-owner 404, update diff detection (`ErrNoUpdates`), delete system-category guard |
| `internal/rules` | 8 | Create, duplicate keyword detection (PostgreSQL 23505 → `ErrDuplicateKeyword`), List, Delete ownership, `CategorizerRules` |
| `internal/transactions` | 13 | Type/date/transfer validation, `ErrTransferNotEditable`, no-fields update, GetByID, List |
| `internal/budgets` | 17 | Period/date validation, end-before-start, 80% warning / 100% exceeded thresholds, inactive budget skips alert |
| `internal/alerts` | 10 | List, MarkRead/MarkAll, Delete, `CreateBudgetAlert` deduplication (calls vs no-calls on `ExistsUnreadForBudget`) |
| `internal/reports` | 14 | Year/month boundary validation, future-month guard, trend window bounds (1–24 months), table-driven validation cases |

### Mock pattern

Every test file implements hand-written mock repositories. The pattern is consistent and simple — one function-field per interface method, safe nil default:

```go
type mockAccountRepo struct {
    createFn func(ctx context.Context, a *Account) error
    // one fn field per interface method
}

func (m *mockAccountRepo) Create(ctx context.Context, a *Account) error {
    if m.createFn != nil { return m.createFn(ctx, a) }
    return nil
}
```

---

## License

Proprietary — ArthaLedger Finance Tracker
