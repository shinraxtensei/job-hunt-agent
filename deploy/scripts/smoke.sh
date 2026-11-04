#!/bin/sh
# Compose smoke test (L-01), run by CI and by `make smoke`: build the images, start the
# stack, check /healthz, /api/leads, the page, the worker and the queue, round-trip a backup through a restore,
# check the backup rotation keeps 14, then stop it and delete its volume.
# It uses its own project name, so it never touches the owner's stack or data.
set -eu
root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"
[ -f .env ] || {
	echo "no .env (CI: cp .env.example .env)" >&2
	exit 1
}
COMPOSE="docker compose -f deploy/compose.yaml --env-file .env -p jobhunt-smoke"
# Its own ports too, so it can run next to the owner's stack.
JOBHUNT_HTTP_PORT=${SMOKE_HTTP_PORT:-18080}
JOBHUNT_DB_PORT=${SMOKE_DB_PORT:-15432}
base=http://127.0.0.1:$JOBHUNT_HTTP_PORT
export COMPOSE JOBHUNT_HTTP_PORT JOBHUNT_DB_PORT
backups=$(mktemp -d "${TMPDIR:-/tmp}/jobhunt-smoke.XXXXXX")
cleanup() {
	$COMPOSE down -v --remove-orphans >/dev/null 2>&1 || true
	case "$backups" in
	*/jobhunt-smoke.*) rm -rf -- "$backups" ;; # only the directory mktemp made
	esac
}
trap cleanup EXIT

GIT_SHA=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) $COMPOSE up -d --build --wait
curl -fsS $base/healthz
echo
curl -fsS "$base/api/leads?limit=1" | grep -q '"leads"' || {
	echo "/api/leads did not answer" >&2
	exit 1
}
echo "leads: ok"
curl -fsS $base/ | grep -q '<div id="root">' || {
	echo "tracker page not served" >&2
	exit 1
}
echo "page: ok"

# The worker (M3-01): healthy (up --wait already waited for its healthcheck), connected
# as the app role, and a SIGTERM stops it cleanly with exit code 0.
[ "$($COMPOSE ps --format '{{.Health}}' worker)" = healthy ] || {
	echo "worker not healthy" >&2
	exit 1
}
$COMPOSE logs worker | grep '"worker started"' | grep -q '"user": "jobhunt_app"' || {
	echo "worker did not start as jobhunt_app" >&2
	exit 1
}
$COMPOSE stop -t 10 worker
code=$(docker inspect -f '{{.State.ExitCode}}' "$($COMPOSE ps -aq worker)")
[ "$code" = 0 ] || {
	echo "worker exited $code on SIGTERM, want 0" >&2
	exit 1
}
$COMPOSE logs worker | grep -q '"worker stopped"'
echo "worker: ok"

# The queue (M3-02): reconcile ran before the worker; counts from the API and the CLI.
$COMPOSE logs reconcile | grep -q '"queue reconciled"' || {
	echo "reconcile did not run" >&2
	exit 1
}
curl -fsS $base/api/queue | grep -q '"pending"' || {
	echo "/api/queue did not answer" >&2
	exit 1
}
$COMPOSE run --rm collector queue | grep -q '^pending '
echo "queue: ok"

# Rotation: 15 older backups plus a new one leave 14.
for i in 01 02 03 04 05 06 07 08 09 10 11 12 13 14 15; do
	touch -t "2026010100$i" "$backups/jobhunt-20260101T00${i}00Z.dump"
done
file=$(JOBHUNT_BACKUP_DIR=$backups deploy/scripts/backup.sh)
n=$(ls -1 "$backups"/jobhunt-*.dump | wc -l | tr -d ' ')
[ "$n" = 14 ] || {
	echo "rotation kept $n backups, want 14" >&2
	exit 1
}
[ -s "$file" ] || {
	echo "empty backup" >&2
	exit 1
}
echo "backup: ok ($n kept, newest $(basename "$file"))"

deploy/scripts/restore.sh "$file"
$COMPOSE up -d --wait api
curl -fsS "$base/api/leads?limit=1" | grep -q '"leads"'
echo "restore: ok"
