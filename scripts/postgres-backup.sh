#!/bin/sh
set -eu
umask 077

interval=${BACKUP_INTERVAL_SECONDS:-86400}
retention=${BACKUP_RETENTION_DAYS:-14}

mkdir -p /backups

while true; do
  timestamp=$(date -u +%Y%m%dT%H%M%SZ)
  partial="/backups/english_learning_${timestamp}.dump.partial"
  final="/backups/english_learning_${timestamp}.dump"
  if pg_dump --format=custom --file="$partial" && pg_restore --list "$partial" >/dev/null; then
    mv "$partial" "$final"
    date +%s > /backups/last-success
    echo "backup verified: $final"
    find /backups -type f -name 'english_learning_*.dump' -mtime "+$retention" -delete
  else
    echo "backup failed; last successful backup remains unchanged" >&2
  fi
  sleep "$interval"
done
