#!/bin/zsh
set -euo pipefail

deploy_host=${VOA_DEPLOY_HOST:-10.10.1.4}
deploy_user=${VOA_DEPLOY_USER:-oldj}
deploy_dir=${VOA_DEPLOY_DIR:-/home/oldj/Application/voa-learning-backend}
target="$deploy_user@$deploy_host"

ssh -o BatchMode=yes "$target" "mkdir -p '$deploy_dir'"
rsync -az --delete --exclude .git --exclude .env --exclude tmp --exclude coverage.out ./ "$target:$deploy_dir/"
ssh "$target" "cd '$deploy_dir' && docker compose config -q && docker compose build api && docker compose up -d --remove-orphans"

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
ssh "$target" "cd '$deploy_dir' && docker compose ps && docker compose logs --tail=80 api worker migrate"
exit 1
