#!/bin/sh
# make restore FILE=… (L-01, M3-05): replaces the stack's database with a backup from
# make backup. The API and the worker are stopped during the restore, then started
# again. The personal files saved with it are never put back silently: this prints how.
set -eu
compose=${COMPOSE:?run through make restore}
file=${1:?usage: make restore FILE=path/to/jobhunt-....dump}
root=$(cd "$(dirname "$0")/../.." && pwd)
[ -f "$file" ] || {
	echo "no such backup: $file" >&2
	exit 1
}

$compose stop api worker
# --clean --if-exists: drop what the dump recreates; one transaction, so a failed
# restore leaves the database as it was.
$compose exec -T postgres pg_restore -U postgres -d jobhunt --clean --if-exists --single-transaction <"$file"
$compose start api worker
echo "restored $file"

personal="${file%.dump}.personal.tar.gz"
if [ -f "$personal" ]; then
	echo
	echo "The personal files saved with this backup (data/profile, agents/evals/postings) were NOT restored;"
	echo "your current ones are untouched. To put them back (this overwrites those files):"
	echo "  tar -xzf '$personal' -C '$root'"
else
	echo "no personal-files archive next to this backup ($personal)"
fi
