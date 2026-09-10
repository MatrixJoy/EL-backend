#!/bin/sh
set -eu

origin=${1:-http://10.10.1.4:8080}
origin=${origin%/}
work_dir=$(mktemp -d)

curl -fsS --max-time 15 "$origin/health/ready" | jq -e '.status == "ready"' >/dev/null
curl -fsS --max-time 15 "$origin/api/v1/bootstrap" | jq -e '.data.anonymous == true' >/dev/null
curl -fsS --max-time 15 "$origin/api/v1/auth/capabilities" | jq -e '(.data.appleSignInEnabled | type) == "boolean" and (.data.developmentSignInEnabled | type) == "boolean"' >/dev/null
curl -fsS --max-time 15 "$origin/api/v1/home" | jq -e '[.data.sections[].items[]] | length > 0' >/dev/null
curl -fsS --max-time 15 "$origin/api/v1/contents" -o "$work_dir/contents.json"
id=$(jq -er '.data[0].id' "$work_dir/contents.json")
title=$(jq -er '.data[0].title' "$work_dir/contents.json")
curl -fsS --max-time 15 "$origin/api/v1/contents/$id" -o "$work_dir/article.json"
jq -e '.data.transcriptBlocks | length > 0' "$work_dir/article.json" >/dev/null
media=$(jq -er '.data.assets[] | select(.kind == "audio") | .playbackUrl' "$work_dir/article.json" | head -1)
case "$media" in /api/v1/media/*) ;; *) echo 'Media is not using the backend proxy' >&2; exit 1 ;; esac
code=$(curl -sS --max-time 20 -H 'Range: bytes=0-1023' -D "$work_dir/media.headers" -o "$work_dir/media.bin" -w '%{http_code}' "$origin$media")
test "$code" = 206
test "$(wc -c < "$work_dir/media.bin" | tr -d ' ')" = 1024
curl -fsS --get --max-time 15 --data-urlencode "q=$title" "$origin/api/v1/search" | jq -e '.data | length > 0' >/dev/null
curl -fsS --max-time 15 "$origin/api/v1/dictionary/learning" | jq -e '.data.entries | length > 0' >/dev/null
curl -fsS --max-time 15 "$origin/api/v1/categories" | jq -e '.data | length > 0' >/dev/null
curl -fsS --max-time 15 "$origin/privacy" -o "$work_dir/privacy.html"
curl -fsS --max-time 15 "$origin/support" -o "$work_dir/support.html"
code=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$origin/api/v1/me")
test "$code" = 401
echo "PASS: $origin — ready, anonymous catalog, detail, timeline, search, dictionary, 206 audio, privacy, support, auth isolation"
echo "Evidence: $work_dir"
