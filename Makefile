.DEFAULT_GOAL := help
SHELL := /bin/bash

# Tool versions are pinned in tools.mod (not go.mod), so the app's dependency graph
# never inherits the linters' dependencies. `$(TOOL) <name>` runs the pinned version
# locally and in CI. Add a tool with: go get -tool -modfile=tools.mod <pkg>@<version>
TOOL := go tool -modfile=tools.mod

# Directories checked by fmt/fmt-check. Overridable so tests can point at a fixture.
GO_DIRS ?= $(shell go list -f '{{.Dir}}' ./...)

# Where `make migration` writes. Overridable so tests can point at a temp dir.
MIGRATIONS_DIR ?= db/migrations

COMPOSE := docker compose -f deploy/compose.yaml --env-file .env

# sqlc's PostgreSQL parser needs cgo, so it runs from a pinned image (ADR-0004).
SQLC_IMAGE := sqlc/sqlc:1.31.1@sha256:70f53171d27b2424e9358869975455a6e955a5aa8e58a998a270a6e34e525537

.PHONY: help check fmt fmt-check vet lint test-go test-integration test-py evals \
	sqlc sqlc-check migration vuln up down fuzz

help: ## List every target
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

# test-integration runs every test (unit + integration), so it replaces test-go here.
check: fmt-check sqlc-check vet lint test-integration vuln ## Everything CI runs (needs Docker)

fmt: ## Format all Go code
	@gofmt -w $(GO_DIRS)

fmt-check: ## Fail if any Go file is not gofmt-clean
	@unformatted=$$(gofmt -l $(GO_DIRS)); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint (errcheck, gosec, revive, gocritic, ...)
	@CGO_ENABLED=0 $(TOOL) golangci-lint run ./...

test-go: ## Go tests with the race detector
	go test -race ./...

test-integration: ## All Go tests plus integration tests against a real Postgres (testcontainers)
	go test -race -tags integration ./...

test-py: ## Python tests
	@echo "test-py: not implemented yet (M3-01)"; exit 1

evals: ## Agent evals
	@echo "evals: not implemented yet (M3-01)"; exit 1

sqlc: ## Regenerate sqlc code from store/queries
	docker run --rm -u "$$(id -u):$$(id -g)" -v "$(CURDIR):/src" -w /src $(SQLC_IMAGE) generate

# Only sqlc's output files, so unstaged edits to queries or tests don't trip it. Compares
# against the index: stage regenerated code before running make check locally.
SQLC_OUT := internal/store/db.go internal/store/models.go 'internal/store/*.sql.go'

sqlc-check: sqlc ## Fail if the committed sqlc code is not what the queries generate
	@git diff --exit-code -- $(SQLC_OUT) || { echo "sqlc output is stale: run make sqlc and commit"; exit 1; }
	@new=$$(git ls-files --others --exclude-standard -- $(SQLC_OUT)); \
	if [ -n "$$new" ]; then echo "sqlc generated untracked files:"; echo "$$new"; exit 1; fi

migration: ## New goose migration: make migration name=<snake_case>
	@[[ "$(name)" =~ ^[a-z0-9_]+$$ ]] || { echo "usage: make migration name=<snake_case>"; exit 1; }
	@f="$(MIGRATIONS_DIR)/$$(date -u +%Y%m%d%H%M%S)_$(name).sql"; \
	printf -- '-- +goose Up\n' > "$$f" && echo "created $$f"

fuzz: ## Fuzz the source parsers for 30s each (not part of check; CI runs the seeds)
	go test -run '^$$' -fuzz=FuzzParse -fuzztime=30s ./internal/source/lever/

vuln: ## Scan dependencies for known vulnerabilities
	$(TOOL) govulncheck ./...

up: ## Start the local stack (Postgres) and wait until it is healthy
	@test -f .env || { echo "no .env: cp .env.example .env and fill it in"; exit 1; }
	$(COMPOSE) up -d --wait

down: ## Stop the local stack; data stays in the named volume
	$(COMPOSE) down
