# Simple Makefile for a Go project

DOCKER_COMPOSE ?= docker compose
DB_SERVICE ?= psql_bp

# Build the application
all: build test

build:
	@echo "Building..."
	@go build -o main.exe cmd/api/main.go

# Run the application
run:
	@go run cmd/api/main.go

# Create DB container
docker-run:
	@docker compose up -d --build

# Shutdown DB container
docker-down:
	@docker compose down

# Test the application
test:
	@echo "Testing..."
	@go test ./... -v

# Integrations Tests for the application
itest:
	@echo "Running integration tests..."
	@RUN_DATABASE_TESTS=1 go test ./internal/database -v

# Create a new SQL migration using goose
migration:
	@if [ -z "$(name)" ]; then echo "Usage: make migration name=NAME"; exit 1; fi
	@cd migrations && goose create $(name) sql

migration-run:
	@goose up

migration-refresh:
	@echo "Recreating public schema..."
	@$(DOCKER_COMPOSE) exec $(DB_SERVICE) sh -lc 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -c "DROP SCHEMA public CASCADE; DROP SCHEMA audit CASCADE; CREATE SCHEMA public;"'
	@echo "Executing migrations..."
	@goose up

db-cli:
	@$(DOCKER_COMPOSE) exec $(DB_SERVICE) sh -lc 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'

# Clean the binary
clean:
	@echo "Cleaning..."
	@rm -f main.exe

# Live Reload
watch:

ifeq ($(OS),Windows_NT)
	@powershell -NoProfile -ExecutionPolicy Bypass -Command "if (Get-Command air -ErrorAction SilentlyContinue) { \
		Write-Output 'Watching...'; \
		air; \
	} else { \
		Write-Output 'air not found in PATH, running via go run...'; \
		go run github.com/air-verse/air@latest; \
	}"
else
	@if command -v air >/dev/null 2>&1; then \
		echo "Watching..."; \
		air; \
	else \
		echo "air not found in PATH, running via go run..."; \
		go run github.com/air-verse/air@latest; \
	fi
endif

.PHONY: all build run test clean watch docker-run docker-down itest migration migration-run migration-refresh db-cli
