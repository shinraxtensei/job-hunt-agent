#!/bin/sh
# make backup (L-01): a timestamped pg_dump in custom format, outside the repository,
# readable only by the owner; keeps the newest 14. Needs the stack running (make up).
#   JOBHUNT_BACKUP_DIR  where backups go (default ~/job-hunter-backups)
set -eu
compose=${COMPOSE:?run through make backup}
dir=${JOBHUNT_BACKUP_DIR:-$HOME/job-hunter-backups}
keep=14

umask 077
mkdir -p "$dir"
chmod 700 "$dir"
file="$dir/jobhunt-$(date -u +%Y%m%dT%H%M%SZ).dump"
# Written under a temporary name so a failed dump never looks like a backup.
$compose exec -T postgres pg_dump -U postgres -d jobhunt --format=custom >"$file.partial"
mv "$file.partial" "$file"

# Keep the newest $keep backups; only files named like this script's are ever removed.
ls -1t "$dir"/jobhunt-*.dump | tail -n +$((keep + 1)) | while IFS= read -r old; do
	rm -f -- "$old"
done
echo "$file"
