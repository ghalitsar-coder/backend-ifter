APP_NAME := api
MODULE   := github.com/ghalitsar-coder/backend-ifter
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

DATABASE_URL ?= postgres://app:app@localhost:5432/app?sslmode=disable

.PHONY: help run build test test-integration fmt fmt-check vet lint tidy check tools db-up db-down migrate-up migrate-down migrate-create sqlc token

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "%-18s %s\n", $$1, $$2}'

run: ## Run the API locally, loading .env when present
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; go run ./cmd/api

build: ## Build the API binary into bin/
	go build -trimpath -ldflags "-s -w -X $(MODULE)/internal/app.Version=$(VERSION)" -o bin/$(APP_NAME) ./cmd/api

test: ## Run all tests (integration tests skip without TEST_DATABASE_URL)
	go test ./...

test-integration: ## Run integration tests against DATABASE_URL
	TEST_DATABASE_URL="$(DATABASE_URL)" go test -count=1 ./...

fmt: ## Format the code
	gofmt -w .

fmt-check: ## Fail when the code is not gofmt-ed
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	golangci-lint run

tidy: ## Tidy go.mod and go.sum
	go mod tidy

check: fmt-check vet test build ## Fast local gate: format, vet, test, build

tools: ## Install local CLI tools
	go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.20.1
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

db-up: ## Start local Postgres
	docker compose up -d postgres

db-down: ## Stop local services
	docker compose down

migrate-up: ## Apply all migrations
	migrate -path db/migrations -database "$(DATABASE_URL)" up

migrate-down: ## Roll back one migration
	migrate -path db/migrations -database "$(DATABASE_URL)" down 1

migrate-create: ## Create a migration: make migrate-create name=add_x
	@if [ -z "$(name)" ]; then echo "usage: make migrate-create name=add_x"; exit 1; fi
	migrate create -ext sql -dir db/migrations -seq "$(name)"

sqlc: ## Regenerate the typed data layer
	sqlc generate

token: ## Mint a dev token: make token SUB=dev ROLE=admin
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; go run ./cmd/token -sub "$(or $(SUB),1)" -roles "$(or $(ROLE),admin)"
