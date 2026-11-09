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
	sqlc sqlc-check migration vuln up down collect backup restore smoke fuzz \
	web-install web-check web-build web-dev evals-replay evals-sample evals-label evals-fit evals-reasons evals-variants evals-tailor evals-checker

help: ## List every target
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  %-18s %s\n", $$1, $$2}'

# test-integration runs every test (unit + integration), so it replaces test-go here.
# web-check runs first so the embedded page is built before the Go build uses it.
check: web-check fmt-check sqlc-check vet lint test-integration test-py evals-replay vuln ## Everything CI runs (needs Docker, Node 26 and uv)

fmt: ## Format all Go code
	@gofmt -w $(GO_DIRS)

fmt-check: ## Fail if any Go file is not gofmt-clean
	@unformatted=$$(gofmt -l $(GO_DIRS)); \
	if [ -n "$$unformatted" ]; then \
		echo "not gofmt-clean:"; echo "$$unformatted"; exit 1; \
	fi

vet: ## Run go vet
	go vet ./...

lint: ## golangci-lint (Go); ruff (lint + format check) and pyright strict (agents/)
	@CGO_ENABLED=0 $(TOOL) golangci-lint run ./...
	cd agents && uv sync --frozen && uv run ruff check . && uv run ruff format --check . && uv run pyright

test-go: ## Go tests with the race detector
	go test -race ./...

test-integration: ## All Go tests plus integration tests against a real Postgres (testcontainers)
	go test -race -tags integration ./...

test-py: ## Python tests, including integration tests against a real Postgres (testcontainers)
	cd agents && uv sync --frozen && uv run pytest

# --- the eval set (M3-07): agents/evals/. Commands that touch the local stack or the
# model read .env (DATABASE_URL on the Mac, JOBHUNT_HTTP_PORT, ANTHROPIC_API_KEY).
EVALS := cd agents && uv run python -m jobhunt_agents.evals
WITH_ENV := set -a && . ./.env && set +a &&

evals: ## Extraction over the eval set, real model, recorded and reported: make evals SET=dev|heldout|all PROMPT=N
	@$(WITH_ENV) $(EVALS) run $(or $(SET),dev) $(or $(PROMPT),1)

evals-fit: ## Coded scores vs the blind fit labels (M3-06), no model call: make evals-fit SET=dev|heldout|all
	@$(WITH_ENV) $(EVALS) fit $(or $(SET),dev)

evals-reasons: ## The reason agent over the eval set, real model, recorded (M3-06 AC3)
	@$(WITH_ENV) $(EVALS) reasons

evals-tailor: ## CV drafts for the good-fit eval postings + injection fixtures, real model (M4-02)
	@$(WITH_ENV) $(EVALS) tailor

evals-checker: ## Claim checker over the last evals-tailor evals-checker drafts + planted guardrail cases, real model (M5-02)
	@$(WITH_ENV) $(EVALS) checker

evals-variants: ## Owner's CV-variant labels vs the model's (M4-06): make evals-variants [SET=init]
	@$(EVALS) variants $(if $(filter init,$(SET)),init,)

evals-replay: ## Re-score recorded eval runs against the labels; no network (CI)
	$(EVALS) replay

evals-sample: ## Draw the eval set from the local stack: 36 shown leads + injection fixtures
	@$(WITH_ENV) $(EVALS) sample

evals-label: ## Label the eval set: make evals-label PASS=fit | prefill | extract
	@case "$(PASS)" in fit) cmd=label-fit ;; prefill) cmd=prefill ;; extract) cmd=label-extract ;; \
		*) echo "usage: make evals-label PASS=fit|prefill|extract"; exit 2 ;; esac; \
	$(WITH_ENV) $(EVALS) $$cmd

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

fuzz: ## Fuzz the source parsers and the normaliser for 30s each (not part of check; CI runs the seeds)
	go test -run '^$$' -fuzz=FuzzParse -fuzztime=30s ./internal/source/lever/
	go test -run '^$$' -fuzz=FuzzParse -fuzztime=30s ./internal/source/greenhouse/
	go test -run '^$$' -fuzz=FuzzParse -fuzztime=30s ./internal/source/ashby/
	go test -run '^$$' -fuzz=FuzzParse -fuzztime=30s ./internal/source/personio/
	go test -run '^$$' -fuzz=FuzzParse -fuzztime=30s ./internal/source/remotive/
	go test -run '^$$' -fuzz=FuzzParse -fuzztime=30s ./internal/source/remoteok/
	go test -run '^$$' -fuzz=FuzzNormalise -fuzztime=30s ./internal/job/
	go test -run '^$$' -fuzz=FuzzTitleKey -fuzztime=30s ./internal/job/
	go test -run '^$$' -fuzz=FuzzClassify -fuzztime=30s ./internal/job/
	go test -run '^$$' -fuzz=FuzzParseBoardLink -fuzztime=30s ./internal/job/

vuln: ## Scan dependencies for known vulnerabilities: govulncheck (Go), uv audit over agents/uv.lock (fails on fixable ones)
	$(TOOL) govulncheck ./...
	cd agents && uv audit --frozen --preview-features audit-command,json-output --output-format json | uv run python tools/audit_fixable.py

up: ## Start the whole stack (Postgres, migrations, API + tracker on http://127.0.0.1:8080)
	@test -f .env || { echo "no .env: cp .env.example .env and fill it in"; exit 1; }
	GIT_SHA=$$(git rev-parse --short HEAD) $(COMPOSE) up -d --build --wait
	@port=$${JOBHUNT_HTTP_PORT:-$$(sed -n 's/^JOBHUNT_HTTP_PORT=//p' .env)}; echo "tracker: http://127.0.0.1:$${port:-8080}"

down: ## Stop the stack; data stays in the named volume
	$(COMPOSE) down

collect: ## Run a collection inside the stack, now (no schedule until milestone D)
	$(COMPOSE) run --rm collector run

backup: ## pg_dump the database to $$JOBHUNT_BACKUP_DIR (default ~/job-hunter-backups), keep 14
	@COMPOSE="$(COMPOSE)" deploy/scripts/backup.sh

restore: ## Restore a backup into the stack: make restore FILE=~/job-hunter-backups/jobhunt-....dump
	@test -n "$(FILE)" || { echo "usage: make restore FILE=path/to/backup.dump"; exit 1; }
	@COMPOSE="$(COMPOSE)" deploy/scripts/restore.sh "$(FILE)"

smoke: ## Compose smoke test in a throwaway project (what CI runs): build, up, check, backup/restore, down
	deploy/scripts/smoke.sh

# --- web/ (M2-06, ADR-0013). Node 26 (web/.nvmrc); npm ci refuses another version.
WEB := npm --prefix web

web-install: ## Install web dependencies exactly as locked (npm ci)
	$(WEB) ci

web-check: web-install ## Web checks: audit, types, lint, unit tests, build, Playwright + axe
	cd web && npm audit --audit-level=high
	$(WEB) run typecheck
	$(WEB) run lint
	$(WEB) test
	$(WEB) run build
	$(WEB) run e2e

web-build: web-install ## Build the tracker page into internal/webui/dist/app (embedded by cmd/api)
	$(WEB) run build

web-dev: web-install ## Vite dev server on :5173, proxying /api to the API on 127.0.0.1:8080
	$(WEB) run dev
