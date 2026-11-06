.DEFAULT_GOAL := help
GO ?= go
GOLANGCI ?= $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
DATABASE_URL ?= postgres://tonnet:tonnet@localhost:5432/tonnet?sslmode=disable
# web/node_modules may contain stray Go packages (e.g. flatted); keep them out of ./...
PKGS := $(shell $(GO) list ./... | grep -v /web/)

.PHONY: help build test test-integration test-e2e lint fmt vet cover run-api run-indexer migrate docker up down logs web-dev contracts-build

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Build both binaries into ./bin
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/api ./cmd/api
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/indexer ./cmd/indexer

test: ## Unit tests with race detector and coverage
	$(GO) test -race -count=1 -coverprofile=coverage.txt -covermode=atomic $(PKGS)

cover: test ## Print per-package coverage
	$(GO) tool cover -func=coverage.txt | tail -1

test-integration: ## Repository tests against PostgreSQL (testcontainers, needs Docker)
	$(GO) test -race -count=1 -tags integration ./internal/repo/... -run Integration -v

test-e2e: ## Read-only smoke test against TON testnet via tonapi
	$(GO) test -count=1 -tags e2e ./internal/chain/ton -run TestE2E -v

lint: ## golangci-lint
	$(GOLANGCI) run $(PKGS)

fmt: ## gofmt all Go files
	gofmt -s -w ./cmd ./internal ./migrations

vet: ## go vet
	$(GO) vet $(PKGS)

run-api: ## Run the API locally (expects .env or exported vars)
	$(GO) run ./cmd/api

run-indexer: ## Run the indexer locally
	$(GO) run ./cmd/indexer

migrate: ## Apply migrations to $$DATABASE_URL (migrations also run on service start)
	$(GO) run github.com/golang-migrate/migrate/v4/cmd/migrate@v4.20.1 -source file://migrations -database "$(DATABASE_URL)" up

docker: ## Build all images
	docker compose build

up: ## Start the full stack
	docker compose up -d --build

down: ## Stop the stack
	docker compose down

logs: ## Tail api + indexer logs
	docker compose logs -f api indexer

web-dev: ## Run the frontend dev server
	cd web && npm run dev

contracts-build: ## Compile FunC contracts with Blueprint
	cd contracts && npm run build
