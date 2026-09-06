#!/bin/zsh
set -euo pipefail

deploy_host=${LEARNING_DEPLOY_HOST:-${VOA_DEPLOY_HOST:-10.10.1.4}}
deploy_user=${LEARNING_DEPLOY_USER:-${VOA_DEPLOY_USER:-oldj}}
deploy_dir=${LEARNING_DEPLOY_DIR:-${VOA_DEPLOY_DIR:-/home/oldj/Application/voa-learning-backend}}
archive_url=${LEARNING_WORDNET_URL:-https://wordnetcode.princeton.edu/wn3.1.dict.tar.gz}
cache_dir=${LEARNING_WORDNET_CACHE_DIR:-${TMPDIR:-/tmp}/english-learning-wordnet}
archive_path="$cache_dir/wn3.1.dict.tar.gz"
target="$deploy_user@$deploy_host"

mkdir -p "$cache_dir"
if [[ ! -s "$archive_path" ]]; then
  curl --fail --location --retry 3 --output "$archive_path" "$archive_url"
fi

remote_machine=$(ssh "$target" uname -m)
case "$remote_machine" in
  x86_64) remote_goarch=amd64 ;;
  aarch64|arm64) remote_goarch=arm64 ;;
  *) echo "unsupported remote architecture: $remote_machine" >&2; exit 1 ;;
esac

build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH="$remote_goarch" go build -trimpath -ldflags="-s -w" -o "$build_dir/dictionary-import" ./cmd/dictionary-import

ssh "$target" "mkdir -p '$deploy_dir/.deploy'"
rsync -az "$build_dir/dictionary-import" "$archive_path" "$target:$deploy_dir/.deploy/"
container_id=$(ssh "$target" "cd '$deploy_dir' && docker compose ps -q api")
ssh "$target" "docker cp '$deploy_dir/.deploy/dictionary-import' '$container_id:/tmp/dictionary-import' && docker cp '$deploy_dir/.deploy/wn3.1.dict.tar.gz' '$container_id:/tmp/wn3.1.dict.tar.gz' && docker exec '$container_id' /tmp/dictionary-import -archive /tmp/wn3.1.dict.tar.gz"
