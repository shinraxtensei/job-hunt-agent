# M1-09: Nightly backup and tested restore

- **Status:** Ready
- **Milestone:** M1 — Collector
- **Linear:** [HAM-28](https://linear.app/hamid-corp/issue/HAM-28/m1-09-nightly-backup-and-tested-restore)
- **Estimate:** ½ day

## Story

As the job seeker, I want the database backed up every night off the server, with a
restore I have actually tried, so that losing the VPS never loses my application history.

## Context

From here on the database holds data worth keeping. An untested backup is not a backup.

## Acceptance criteria

1. **Given** the server
   **When** 03:00 arrives
   **Then** `pg_dump` (custom format) runs, the file is encrypted, and it is uploaded to
   off-server object storage.
2. **Given** backups older than 14 daily / 8 weekly
   **When** the job runs
   **Then** they are pruned by the retention rule.
3. **Given** the latest backup
   **When** `deploy/restore.sh` is run against a fresh local Postgres
   **Then** the database restores and row counts match the source; the procedure and the
   date of the test are written in `deploy/README.md`.
4. **Given** a backup failure
   **When** it happens
   **Then** a healthchecks.io check alerts the owner.

## Out of scope

- Point-in-time recovery.

## Security notes

Backups contain personal data from M4 onward: encrypted before upload; the key is stored
outside the server and outside the repo. Storage credentials are write-only where the
provider allows it.
