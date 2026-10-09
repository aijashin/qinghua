#!/usr/bin/env bash
set -euo pipefail
project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$project_root"
if [[ -f .env ]]; then
  set -a
  # .env is an operator-owned shell environment file; keep it private.
  source .env
  set +a
fi
export ZHIWEI_DATA_DIR="${ZHIWEI_DATA_DIR:-$project_root/data}"
export ZHIWEI_WEB_DIR="${ZHIWEI_WEB_DIR:-$project_root/dist}"
if [[ "$ZHIWEI_DATA_DIR" != /* ]]; then export ZHIWEI_DATA_DIR="$project_root/$ZHIWEI_DATA_DIR"; fi
if [[ "$ZHIWEI_WEB_DIR" != /* ]]; then export ZHIWEI_WEB_DIR="$project_root/$ZHIWEI_WEB_DIR"; fi
if [[ ! -x "$project_root/bin/zhiwei" ]]; then
  echo "Server binary is missing. Run: make backend-build" >&2
  exit 1
fi
exec "$project_root/bin/zhiwei" "$@"
