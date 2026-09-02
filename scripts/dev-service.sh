#!/bin/zsh
set -euo pipefail

root_dir=${0:A:h:h}
runtime_dir="$root_dir/tmp/dev-service"
domain="gui/$(id -u)"
api_label="com.oldj.voa-learning.api"
worker_label="com.oldj.voa-learning.worker"

service_loaded() {
  launchctl print "$domain/$1" >/dev/null 2>&1
}

write_plist() {
  local label=$1 executable=$2 output=$3 plist=$4
  local escaped_root=${root_dir//&/&amp;}
  local escaped_executable=${executable//&/&amp;}
  local escaped_output=${output//&/&amp;}
  cat >"$plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>$label</string>
  <key>ProgramArguments</key><array><string>$escaped_executable</string></array>
  <key>WorkingDirectory</key><string>$escaped_root</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ThrottleInterval</key><integer>3</integer>
  <key>StandardOutPath</key><string>$escaped_output</string>
  <key>StandardErrorPath</key><string>$escaped_output</string>
</dict></plist>
PLIST
}

unload() {
  local label=$1
  service_loaded "$label" && launchctl bootout "$domain/$label"
}

start() {
  mkdir -p "$runtime_dir"
  go build -o "$runtime_dir/api" ./cmd/api
  go build -o "$runtime_dir/worker" ./cmd/worker
  write_plist "$api_label" "$runtime_dir/api" "$runtime_dir/api.log" "$runtime_dir/api.plist"
  write_plist "$worker_label" "$runtime_dir/worker" "$runtime_dir/worker.log" "$runtime_dir/worker.plist"
  unload "$api_label" || true
  unload "$worker_label" || true
  launchctl bootstrap "$domain" "$runtime_dir/api.plist"
  launchctl bootstrap "$domain" "$runtime_dir/worker.plist"

  for _ in {1..40}; do
    if curl -fsS http://127.0.0.1:8080/health/ready >/dev/null 2>&1; then
      echo "backend ready at http://127.0.0.1:8080"
      return
    fi
    sleep 0.25
  done
  echo "backend failed to become ready; see $runtime_dir/api.log" >&2
  return 1
}

stop() {
  unload "$worker_label" || true
  unload "$api_label" || true
  echo "backend services stopped"
}

status() {
  service_loaded "$api_label" && echo "API supervised by launchd" || echo "API stopped"
  service_loaded "$worker_label" && echo "worker supervised by launchd" || echo "worker stopped"
  curl -fsS http://127.0.0.1:8080/health/ready 2>/dev/null || true
}

case ${1:-start} in
  start) start ;;
  stop) stop ;;
  restart) stop; start ;;
  status) status ;;
  *) echo "usage: $0 {start|stop|restart|status}" >&2; exit 2 ;;
esac
