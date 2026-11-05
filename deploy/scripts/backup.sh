#!/bin/sh
# make backup (L-01, M3-05): a timestamped pg_dump in custom format and, with the same
# timestamp, an archive of the git-ignored personal files (data/profile and the eval
# set's posting texts, agents/evals/postings), outside the repository, readable only by the owner; keeps the
# newest 14 of each. Needs the stack running (make up). Prints the dump's path.
#   JOBHUNT_BACKUP_DIR  where backups go (default ~/job-hunter-backups)
set -eu
compose=${COMPOSE:?run through make backup}
dir=${JOBHUNT_BACKUP_DIR:-$HOME/job-hunter-backups}
keep=14
root=$(cd "$(dirname "$0")/../.." && pwd)

umask 077
mkdir -p "$dir"
chmod 700 "$dir"
stamp=$(date -u +%Y%m%dT%H%M%SZ)
file="$dir/jobhunt-$stamp.dump"
personal="$dir/jobhunt-$stamp.personal.tar.gz"

# Written under temporary names so a failed run never looks like a backup.
$compose exec -T postgres pg_dump -U postgres -d jobhunt --format=custom >"$file.partial"
mv "$file.partial" "$file"

set --
for p in data/profile agents/evals/postings; do
	if [ -e "$root/$p" ]; then set -- "$@" "$p"; fi
done
if [ $# -gt 0 ]; then
	tar -czf "$personal.partial" -C "$root" "$@"
else
	tar -czf "$personal.partial" -T /dev/null # nothing personal yet: an empty archive
fi
mv "$personal.partial" "$personal"
echo "personal files: $personal" >&2

# Keep the newest $keep of each kind; only files named like this script's are removed.
rotate() {
	ls -1t "$dir"/jobhunt-*"$1" | tail -n +$((keep + 1)) | while IFS= read -r old; do
		rm -f -- "$old"
	done
}
rotate .dump
rotate .personal.tar.gz
echo "$file"
