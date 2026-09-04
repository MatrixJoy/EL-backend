#!/bin/zsh
set -euo pipefail

deploy_host=${LEARNING_DEPLOY_HOST:-${VOA_DEPLOY_HOST:-10.10.1.4}}
deploy_user=${LEARNING_DEPLOY_USER:-${VOA_DEPLOY_USER:-oldj}}
# Keep the existing server directory as a compatibility location until storage is migrated.
deploy_dir=${LEARNING_DEPLOY_DIR:-${VOA_DEPLOY_DIR:-/home/oldj/Application/voa-learning-backend}}
target="$deploy_user@$deploy_host"

ssh -o BatchMode=yes "$target" "mkdir -p '$deploy_dir'"
rsync -az --delete --exclude .git --exclude .env --exclude tmp --exclude coverage.out ./ "$target:$deploy_dir/"
ssh "$target" "cd '$deploy_dir' && docker compose config -q"

if ! ssh "$target" "cd '$deploy_dir' && docker compose build api"; then
  echo "registry build unavailable; falling back to locally compiled Linux binaries"
  remote_machine=$(ssh "$target" uname -m)
  case "$remote_machine" in
    x86_64) remote_goarch=amd64 ;;
    aarch64|arm64) remote_goarch=arm64 ;;
    *) echo "unsupported remote architecture: $remote_machine" >&2; exit 1 ;;
  esac
  build_dir=$(mktemp -d)
  trap 'rm -rf "$build_dir"' EXIT
  CGO_ENABLED=0 GOOS=linux GOARCH="$remote_goarch" go build -trimpath -ldflags="-s -w" -o "$build_dir/api" ./cmd/api
  CGO_ENABLED=0 GOOS=linux GOARCH="$remote_goarch" go build -trimpath -ldflags="-s -w" -o "$build_dir/worker" ./cmd/worker
  ssh "$target" "mkdir -p '$deploy_dir/.deploy'"
  rsync -az "$build_dir/api" "$build_dir/worker" "$target:$deploy_dir/.deploy/"
  ssh "$target" "cd '$deploy_dir' && docker build --pull=false -f Dockerfile.prebuilt -t voa-learning-backend:local ."
fi

ssh "$target" "cd '$deploy_dir' && docker compose up -d --remove-orphans"

for _ in {1..40}; do
  if curl -fsS "http://$deploy_host:8080/health/ready" >/dev/null 2>&1; then
    echo "deployed: http://$deploy_host:8080"
    curl -fsS "http://$deploy_host:8080/health/ready"
    echo
    exit 0
  fi
  sleep 0.5
done

echo "deployment did not become ready" >&2
ssh "$target" "cd '$deploy_dir' && docker compose ps && docker compose logs --tail=80 api migrate"
exit 1
