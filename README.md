# ArthaLedger — Personal Finance Tracker API

A production-quality REST API backend for personal finance management, built with Go. Automatically categorizes daily transactions and provides spending insights, budget tracking, and financial reports.

---

## Overview

ArthaLedger helps users track income and expenses across multiple bank accounts. Transactions are categorized automatically using keyword rules, with an optional OpenAI fallback for unknown merchants. The system supports budgeting, smart alerts, and monthly financial reports.

**Status:** Phase 2 complete — accounts & transactions API live with atomic balance management, transfer support, and Swagger UI.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.22+ |
| HTTP Framework | Gin |
| ORM | GORM + pgx driver |
| Database | PostgreSQL 18 |
| Cache | Redis 7 |
| Configuration | Viper (reads `.env`) |
| Migrations | golang-migrate |
| Auth | JWT HS256 (`golang-jwt/jwt/v5`) |
| Logging | `log/slog` (stdlib) |
| Containerization | Docker & Docker Compose |

---

## Project Structure

```
ArthaLedger/
├── cmd/server/
│   └── main.go                 # Entry point — wires dependencies, starts server
├── config/
│   └── config.go               # Loads .env into typed Config struct via Viper
├── internal/                   # Business logic (one sub-package per domain)
│   ├── auth/                   # User registration, login, JWT, logout
│   ├── accounts/               # Bank account CRUD
│   ├── transactions/           # Transaction CRUD, CSV import, categorization
│   ├── categories/             # System and user-defined categories
│   ├── budgets/                # Monthly budget tracking
│   ├── reports/                # Monthly summaries, trends, CSV export
│   └── alerts/                 # Budget warnings and unusual spend detection
├── pkg/                        # Shared, reusable packages
│   ├── database/
│   │   ├── postgres.go         # Opens GORM connection with connection pool config
│   │   └── redis.go            # Creates Redis client with timeouts
│   ├── middleware/             # Auth, rate limiting, request logging
│   ├── categorizer/            # Keyword rules engine + OpenAI fallback
│   └── mailer/                 # SMTP email notifications
├── migrations/
│   ├── 000001_init_schema.up.sql   # Creates all tables and seeds system categories
│   └── 000001_init_schema.down.sql # Drops all tables
├── docker-compose.yml          # PostgreSQL + Redis services
├── Dockerfile                  # Two-stage build (builder → alpine:3.19)
├── Makefile                    # Developer shortcuts (see below)
├── .env.example                # Config template — copy to .env and fill in values
└── go.mod / go.sum             # Go module definition and verified dependency hashes
```

### Architecture pattern

Every domain in `internal/` follows a strict three-layer pattern:

```
Handler  →  Service  →  Repository
```

- **Handler** — parses HTTP request, calls service, writes HTTP response. No business logic.
- **Service** — owns all business rules. No SQL.
- **Repository** — owns all database queries. Always accepts `context.Context` as the first argument.

Each layer depends only on interfaces, never on concrete types. DB and Redis clients are injected via constructors, never stored as globals.

---

## API

All routes are prefixed with `/api/v1`. All routes except `/auth/register` and `/auth/login` require an `Authorization: Bearer <token>` header.

| Group | Prefix |
|---|---|
| Auth | `/api/v1/auth` |
| Accounts | `/api/v1/accounts` |
| Transactions | `/api/v1/transactions` |
| Categories | `/api/v1/categories` |
| Budgets | `/api/v1/budgets` |
| Reports | `/api/v1/reports` |
| Alerts | `/api/v1/alerts` |
| Health | `/health` |

Full Swagger UI is available at `http://localhost:8080/swagger/index.html` after running `make swagger`. Re-run after any annotation change.

All responses use one of two shapes:
```json
{ "success": true,  "data":  <payload> }
{ "success": false, "error": "<message>" }
```

---

## Database Schema

Six tables — created by running `make migrate-up`.

| Table | Purpose |
|---|---|
| `users` | Accounts with bcrypt-hashed passwords |
| `accounts` | Bank/cash/credit accounts per user |
| `categories` | System-wide (seeded) and user-defined categories |
| `transactions` | Income and expense records linked to an account |
| `budgets` | Monthly spending limits per category |
| `alerts` | Budget warnings, breach notices, unusual spend flags |

16 system categories are seeded automatically (Salary, Food & Dining, Transport, Shopping, etc.).

---

## Quick Start

### Prerequisites

- Go 1.22+
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

Copy `.env.example` to `.env`. The application reads all settings from this file via Viper — no values are hardcoded in source.

Key settings: `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD`, `REDIS_HOST`, `REDIS_PORT`, `JWT_SECRET` (minimum 32 characters), `APP_ENV` (`development` or `production`).

`OPENAI_API_KEY` is optional — if empty, the AI categorization step is skipped and unknown transactions default to "Others".

---

## License

Proprietary — ArthaLedger Finance Tracker
