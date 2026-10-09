#!/usr/bin/env bash
set -euo pipefail
project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"
api_pid=""
web_pid=""
cleanup() {
  trap - EXIT INT TERM
  if [[ -n "$api_pid" ]]; then kill "$api_pid" 2>/dev/null || true; fi
  if [[ -n "$web_pid" ]]; then kill "$web_pid" 2>/dev/null || true; fi
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM
./scripts/server.sh serve &
api_pid=$!
npm run dev -- --host 127.0.0.1 &
web_pid=$!
# macOS ships Bash 3, which has no `wait -n`.
while kill -0 "$api_pid" 2>/dev/null && kill -0 "$web_pid" 2>/dev/null; do sleep 1; done
echo "A development service stopped. See its output above." >&2
exit 1
