include .env
export

.PHONY: help run build test test-cover migrate-up migrate-down swagger docker-up docker-down docker-logs lint tidy clean

help:
	@echo "ArthaLedger - Finance Tracker API"
	@echo ""
	@echo "Available targets:"
	@echo "  make run              - Run the server in development mode"
	@echo "  make build            - Build the binary"
	@echo "  make test             - Run all tests with verbose output"
	@echo "  make test-cover       - Run tests with coverage and open report"
	@echo "  make migrate-up       - Run all pending migrations"
	@echo "  make migrate-down     - Rollback last migration"
	@echo "  make swagger          - Generate Swagger documentation"
	@echo "  make docker-up        - Start PostgreSQL and Redis containers"
	@echo "  make docker-down      - Stop and remove containers"
	@echo "  make docker-logs      - Stream container logs"
	@echo "  make lint             - Run golangci-lint"
	@echo "  make tidy             - Cleanup go.mod and go.sum"
	@echo "  make clean            - Remove build artifacts"

# Development targets
run:
	go run ./cmd/server/main.go

build:
	mkdir -p bin
	go build -o bin/server ./cmd/server/main.go

test:
	go test ./... -v

test-cover:
	go test ./... -coverprofile=coverage.out -v
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Database migration targets
migrate-up:
	migrate -path migrations -database "postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}" up

migrate-down:
	migrate -path migrations -database "postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}" down 1

# API Documentation
swagger:
	swag init -g ./cmd/server/main.go -o ./docs

# Docker targets
docker-up:
	docker-compose up -d
	@echo "Docker containers started. Waiting for services to be ready..."
	@sleep 3
	@echo "PostgreSQL and Redis are now running"

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f

# Code quality targets
lint:
	golangci-lint run ./...

tidy:
	go mod tidy

clean:
	rm -rf bin/
	rm -rf coverage.out coverage.html
	@echo "Build artifacts cleaned"

# Default target
.DEFAULT_GOAL := help
