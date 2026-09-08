#!/bin/zsh
set -euo pipefail

deploy_host=${LEARNING_DEPLOY_HOST:-${VOA_DEPLOY_HOST:-10.10.1.4}}
deploy_user=${LEARNING_DEPLOY_USER:-${VOA_DEPLOY_USER:-oldj}}
deploy_dir=${LEARNING_DEPLOY_DIR:-${VOA_DEPLOY_DIR:-/home/oldj/Application/voa-learning-backend}}
deploy_port=${LEARNING_DEPLOY_SSH_PORT:-22}
compose_file=${LEARNING_DEPLOY_COMPOSE_FILE:-compose.yaml}
compose_env_file=${LEARNING_DEPLOY_ENV_FILE:-}
archive_url=${LEARNING_WORDNET_URL:-https://wordnetcode.princeton.edu/wn3.1.dict.tar.gz}
cache_dir=${LEARNING_WORDNET_CACHE_DIR:-${TMPDIR:-/tmp}/english-learning-wordnet}
archive_path="$cache_dir/wn3.1.dict.tar.gz"
target="$deploy_user@$deploy_host"
ssh_command=(ssh -p "$deploy_port")
rsync_shell="ssh -p $deploy_port"

mkdir -p "$cache_dir"
if [[ ! -s "$archive_path" ]]; then
  curl --fail --location --retry 3 --output "$archive_path" "$archive_url"
fi

remote_machine=$($ssh_command "$target" uname -m)
case "$remote_machine" in
  x86_64) remote_goarch=amd64 ;;
  aarch64|arm64) remote_goarch=arm64 ;;
  *) echo "unsupported remote architecture: $remote_machine" >&2; exit 1 ;;
esac

build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH="$remote_goarch" go build -trimpath -ldflags="-s -w" -o "$build_dir/dictionary-import" ./cmd/dictionary-import

$ssh_command "$target" "mkdir -p '$deploy_dir/.deploy'"
rsync -az -e "$rsync_shell" "$build_dir/dictionary-import" "$archive_path" "$target:$deploy_dir/.deploy/"
if [[ -n "$compose_env_file" ]]; then
  container_id=$($ssh_command "$target" "cd '$deploy_dir' && docker compose --env-file '$compose_env_file' -f '$compose_file' ps -q api")
else
  container_id=$($ssh_command "$target" "cd '$deploy_dir' && docker compose -f '$compose_file' ps -q api")
fi
if [[ -z "$container_id" ]]; then
  echo "API container is not running" >&2
  exit 1
fi
$ssh_command "$target" "docker cp '$deploy_dir/.deploy/dictionary-import' '$container_id:/tmp/dictionary-import' && docker cp '$deploy_dir/.deploy/wn3.1.dict.tar.gz' '$container_id:/tmp/wn3.1.dict.tar.gz' && docker exec '$container_id' /tmp/dictionary-import -archive /tmp/wn3.1.dict.tar.gz"
