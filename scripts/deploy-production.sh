#!/bin/zsh
set -euo pipefail

script_dir=${0:A:h}
repo_dir=${script_dir:h}
deploy_host=${LEARNING_PRODUCTION_HOST:-106.53.192.46}
deploy_port=${LEARNING_PRODUCTION_SSH_PORT:-36987}
deploy_user=${LEARNING_PRODUCTION_USER:-oldj}
deploy_dir=${LEARNING_PRODUCTION_DIR:-/opt/english-learning-backend}
gateway_dir=${LEARNING_GATEWAY_DIR:-/opt/lighttech_deploy/gateway}
tls_email=${LEARNING_TLS_EMAIL:-zlogo913@gmail.com}
target="$deploy_user@$deploy_host"
ssh_command=(ssh -p "$deploy_port" -o BatchMode=yes)
rsync_shell="ssh -p $deploy_port -o BatchMode=yes"

cd "$repo_dir"

remote_machine=$($ssh_command "$target" uname -m)
case "$remote_machine" in
  x86_64) remote_goarch=amd64 ;;
  aarch64|arm64) remote_goarch=arm64 ;;
  *) echo "unsupported remote architecture: $remote_machine" >&2; exit 1 ;;
esac

build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT
CGO_ENABLED=0 GOOS=linux GOARCH="$remote_goarch" go build -trimpath -ldflags="-s -w" -o "$build_dir/api" ./cmd/api
CGO_ENABLED=0 GOOS=linux GOARCH="$remote_goarch" go build -trimpath -ldflags="-s -w" -o "$build_dir/worker" ./cmd/worker

$ssh_command "$target" "sudo -n mkdir -p '$deploy_dir' && sudo -n chown '$deploy_user' '$deploy_dir' && mkdir -p '$deploy_dir/.deploy' '$deploy_dir/backups'"
rsync -az --delete -e "$rsync_shell" \
  --exclude .git --exclude .env --exclude .env.production --exclude .deploy --exclude tmp --exclude coverage.out \
  --exclude backups --exclude data \
  ./ "$target:$deploy_dir/"
rsync -az -e "$rsync_shell" "$build_dir/api" "$build_dir/worker" "$target:$deploy_dir/.deploy/"

$ssh_command "$target" "cd '$deploy_dir' && chmod 700 scripts/init-production-env.sh && chmod 755 scripts/postgres-backup.sh && ./scripts/init-production-env.sh /opt/lighttech_deploy/.env .env.production && docker compose --env-file .env.production -f compose.production.yaml config -q && docker compose --env-file .env.production -f compose.production.yaml build api && docker compose --env-file .env.production -f compose.production.yaml up -d --remove-orphans"

rsync -az -e "$rsync_shell" deploy/nginx/ela.wozdou.cn.http.conf "$target:$deploy_dir/ela.wozdou.cn.http.conf"
rsync -az -e "$rsync_shell" deploy/nginx/ela.wozdou.cn.conf "$target:$deploy_dir/ela.wozdou.cn.conf"

$ssh_command "$target" "
  set -eu
  if ! sudo -n test -s '$gateway_dir/certbot/conf/live/ela.wozdou.cn/fullchain.pem'; then
    cp '$deploy_dir/ela.wozdou.cn.http.conf' '$gateway_dir/conf.d/ela.wozdou.cn.conf'
    docker exec lighttech-nginx nginx -t
    docker exec lighttech-nginx nginx -s reload
    cd '$gateway_dir'
    docker compose run --rm --entrypoint certbot certbot certonly --webroot -w /var/www/certbot -d ela.wozdou.cn --email '$tls_email' --agree-tos --non-interactive
  fi
  cp '$deploy_dir/ela.wozdou.cn.conf' '$gateway_dir/conf.d/ela.wozdou.cn.conf'
  docker exec lighttech-nginx nginx -t
  docker exec lighttech-nginx nginx -s reload
  cd '$gateway_dir'
  docker compose up -d certbot
"

for _ in {1..60}; do
  if curl -fsS --max-time 5 https://ela.wozdou.cn/health/ready >/dev/null 2>&1; then
    echo "deployed: https://ela.wozdou.cn"
    curl -fsS https://ela.wozdou.cn/health/ready
    echo
    exit 0
  fi
  sleep 1
done

echo "production deployment did not become ready" >&2
$ssh_command "$target" "cd '$deploy_dir' && docker compose --env-file .env.production -f compose.production.yaml ps && docker compose --env-file .env.production -f compose.production.yaml logs --tail=120 api migrate postgres redis"
exit 1
