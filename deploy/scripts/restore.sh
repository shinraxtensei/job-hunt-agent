#!/bin/sh
# make restore FILE=… (L-01): replaces the stack's database with a backup from
# make backup. The API is stopped during the restore, then started again.
set -eu
compose=${COMPOSE:?run through make restore}
file=${1:?usage: make restore FILE=path/to/jobhunt-....dump}
[ -f "$file" ] || {
	echo "no such backup: $file" >&2
	exit 1
}

$compose stop api
# --clean --if-exists: drop what the dump recreates; one transaction, so a failed
# restore leaves the database as it was.
$compose exec -T postgres pg_restore -U postgres -d jobhunt --clean --if-exists --single-transaction <"$file"
$compose start api
echo "restored $file"
