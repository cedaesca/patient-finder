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

# --- Production remote ops ------------------------------------------------
PROD_REMOTE_DIR ?= /opt/apps/patient-finder
PROD_EXCEL ?= pacientes.xlsx

-include .env

require-prod-host:
	@if [ -z "$(PROD_SSH_HOST)" ]; then \
		echo "Error: PROD_SSH_HOST no esta definida en .env (ej. PROD_SSH_HOST=lupicrm-vps-deploy)"; \
		exit 1; \
	fi

prod-scp-excel: require-prod-host
	@scp "$(PROD_EXCEL)" "$(PROD_SSH_HOST):$(PROD_REMOTE_DIR)/"

prod-import: require-prod-host
	@ssh "$(PROD_SSH_HOST)" "cd $(PROD_REMOTE_DIR) && docker compose run --rm import"

prod-search-reindex: require-prod-host
	@if [ -z "$(COLLECTION)" ]; then echo "Usage: make prod-search-reindex COLLECTION=persons"; exit 1; fi
	@ssh "$(PROD_SSH_HOST)" "cd $(PROD_REMOTE_DIR) && docker compose exec -T api /app/api search:reindex $(COLLECTION)"

prod-search-reindex-all: require-prod-host
	@ssh "$(PROD_SSH_HOST)" "cd $(PROD_REMOTE_DIR) && docker compose exec -T api /app/api search:reindex --all"

prod-compose-config:
	@docker compose -f deploy/prod/docker-compose.prod.yaml config >/dev/null && echo "OK: deploy/prod/docker-compose.prod.yaml is valid"

.PHONY: all build run test clean watch docker-run docker-down itest migration migration-run migration-refresh db-cli prod-scp-excel prod-import prod-search-reindex prod-search-reindex-all prod-compose-config require-prod-host
