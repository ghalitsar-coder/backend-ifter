DATABASE_URL ?= postgres://ifter:ifter@localhost:5433/ifter?sslmode=disable
DB_USER ?= ifter
DB_NAME ?= ifter
BACKUP_DIR ?= backups

.PHONY: dev run build test test-integration e2e fmt vet lint tidy db-up db-down tools migrate-up migrate-down migrate-create sqlc backup restore verify-phase security

dev:
	air

run:
	go run ./cmd/api

build:
	go build -trimpath -o bin/api ./cmd/api

test:
	go test ./...

test-integration:
	TEST_DATABASE_URL="$(DATABASE_URL)" go test -count=1 ./internal/... -v

e2e:
	./scripts/e2e.sh

fmt:
	gofmt -w .

vet:
	go vet ./...

lint:
	golangci-lint run

tidy:
	go mod tidy

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

tools:
	go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@v4.20.1
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

migrate-up:
	migrate -path db/migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path db/migrations -database "$(DATABASE_URL)" down 1

migrate-create:
	@if [ -z "$(name)" ]; then echo "usage: make migrate-create name=add_x"; exit 1; fi
	migrate create -ext sql -dir db/migrations -seq "$(name)"

sqlc:
	sqlc generate

backup:
	mkdir -p "$(BACKUP_DIR)"
	docker compose exec -T postgres pg_dump -U "$(DB_USER)" -d "$(DB_NAME)" -Fc > "$(BACKUP_DIR)/$(DB_NAME)-$$(date +%Y%m%d-%H%M%S).dump"

restore:
	@if [ -z "$(file)" ]; then echo "usage: make restore file=backups/ifter-YYYYmmdd-HHMMSS.dump"; exit 1; fi
	docker compose exec -T postgres pg_restore -U "$(DB_USER)" -d "$(DB_NAME)" --clean --if-exists --no-owner < "$(file)"

verify-phase:
	@if [ -z "$(PHASE)" ]; then echo "usage: make verify-phase PHASE=1|2|3|4|5|6"; exit 1; fi
	@case "$(PHASE)" in \
		1|foundation) pkgs="./internal/auth/... ./internal/config/... ./internal/transport/http/... ./internal/platform/database/..." ;; \
		2|master-data) pkgs="./internal/masterdata/..." ;; \
		3|imports) pkgs="./internal/imports/..." ;; \
		4|attendance) pkgs="./internal/attendance/..." ;; \
		5|discipline) pkgs="./internal/discipline/..." ;; \
		6|reporting) pkgs="./internal/attendance/... ./internal/reporting/... ./internal/platform/metrics/... ./internal/transport/http/..." ;; \
		*) echo "unknown PHASE=$(PHASE)"; exit 1 ;; \
	esac; \
	echo "ECC verification phase $(PHASE): $$pkgs"; \
	test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }; \
	go build ./... && \
	go vet $$pkgs && \
	TEST_DATABASE_URL="$(DATABASE_URL)" go test -count=1 -race -cover $$pkgs && \
	golangci-lint run $$pkgs

security:
	go mod verify
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
