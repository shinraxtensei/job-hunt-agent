.DEFAULT_GOAL := help
SHELL := /bin/bash

# Tool versions are pinned in tools.mod (not go.mod), so the app's dependency graph
# never inherits the linters' dependencies. `$(TOOL) <name>` runs the pinned version
# locally and in CI. Add a tool with: go get -tool -modfile=tools.mod <pkg>@<version>
TOOL := go tool -modfile=tools.mod

# Directories checked by fmt/fmt-check. Overridable so tests can point at a fixture.
GO_DIRS ?= $(shell go list -f '{{.Dir}}' ./...)

.PHONY: help check fmt fmt-check vet lint test-go test-integration test-py evals \
	sqlc migration vuln up down

help: ## List every target
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

check: fmt-check vet lint test-go vuln ## Everything CI runs

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

test-integration: ## Go integration tests against a real Postgres (testcontainers)
	@echo "test-integration: not implemented yet (M0-03)"; exit 1

test-py: ## Python tests
	@echo "test-py: not implemented yet (M3-01)"; exit 1

evals: ## Agent evals
	@echo "evals: not implemented yet (M3-01)"; exit 1

sqlc: ## Regenerate sqlc code from store/queries
	@echo "sqlc: not implemented yet (M0-03)"; exit 1

migration: ## New goose migration: make migration name=<snake_case>
	@echo "migration: not implemented yet (M0-03)"; exit 1

vuln: ## Scan dependencies for known vulnerabilities
	$(TOOL) govulncheck ./...

up: ## Start the local stack
	@echo "up: not implemented yet (M0-03)"; exit 1

down: ## Stop the local stack
	@echo "down: not implemented yet (M0-03)"; exit 1
