# M0-01: Go module and tooling

- **Status:** Ready
- **Milestone:** M0 — Walking skeleton
- **Linear:** —
- **Estimate:** ½ day

## Story

As the job seeker building this tool, I want one command that formats, lints, tests and
scans the code, so that every change is checked the same way locally and in CI.

## Context

First code in the repo. Everything later relies on `make check` being the single
definition of "green". See CLAUDE.md → Commands.

## Acceptance criteria

1. **Given** a fresh clone with Go installed
   **When** I run `make check`
   **Then** it runs `gofmt` check, `go vet`, golangci-lint, `go test -race ./...` and
   `govulncheck ./...`, and exits 0 on the empty project.
2. **Given** a Go file with a formatting error or an unchecked error return
   **When** I run `make check`
   **Then** it exits non-zero and names the file and rule.
3. **Given** the repo
   **When** I read `go.mod`
   **Then** the module path is `github.com/shinraxtensei/job-hunt-agent` and the Go
   version is the current stable release.
4. **Given** the repo
   **When** I run `make help`
   **Then** every target in the CLAUDE.md Commands table is listed with a one-line
   description (targets not built yet print "not implemented yet" and exit 1).

## Out of scope

- Python tooling (M3-01), TypeScript tooling (M2-06), Docker (M0-03).

## Security notes

govulncheck is part of `check` from day one. golangci-lint enables `gosec` and
`errcheck`. No secrets involved.

## Notes for the implementer

- Pin tool versions with `go tool` directives in `go.mod` (Go 1.24+), so CI and local
  use the same versions.
- golangci-lint config: start from the defaults plus `errcheck`, `gosec`, `revive`,
  `gocritic`, `bodyclose`, `contextcheck`, `errorlint`. Keep the list short.
