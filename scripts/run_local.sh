#!/usr/bin/env bash
# 本地一键开发启动（不依赖 Docker）：样本站点 + 后端服务（自动迁移/种子/采集）
# 用法：
#   ./scripts/run_local.sh            # 启动样本站点与后端，前端用内置 dist（需先 npm run build）
#   ./scripts/run_local.sh dev        # 额外启动 Vite 开发服务器（5173，热更新）
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO=/opt/go/bin/go
NPM=/opt/node-22/bin/npm
PSQL=/opt/postgresql-18/bin/psql
export DATABASE_URL="${DATABASE_URL:-postgres://ccdev:ccdev@127.0.0.1:5432/ccdev?sslmode=disable}"
export SAMPLE_BASE_URL="${SAMPLE_BASE_URL:-http://127.0.0.1:9090}"
export POLL_INTERVAL="${POLL_INTERVAL:-1s}"
export LEASE_DURATION="${LEASE_DURATION:-15s}"
export REVIEW_INTERVAL="${REVIEW_INTERVAL:-90s}"

echo "==> 构建后端"
(cd "$ROOT/backend" && "$GO" build -o /tmp/pw-server ./cmd/server && "$GO" build -o /tmp/pw-sample ./cmd/sampleserver)

if [ "${1:-}" = "dev" ]; then
  echo "==> 安装并构建前端"
  (cd "$ROOT/frontend" && "$NPM" install --cache "$ROOT/frontend/.npm-cache" && "$NPM" run build)
fi

echo "==> 启动样本站点 :9090"
pkill -f /tmp/pw-sample 2>/dev/null || true
/tmp/pw-sample -addr :9090 >/tmp/pw-sample.log 2>&1 &
echo $! >/tmp/pw-sample.pid

echo "==> 启动主服务 :8080（启动时自动执行迁移与种子）"
pkill -f /tmp/pw-server 2>/dev/null || true
FRONTEND_DIST="$ROOT/frontend/dist" /tmp/pw-server >/tmp/pw-server.log 2>&1 &
echo $! >/tmp/pw-server.pid

echo "样本站点: http://127.0.0.1:9090  （健康检查 /healthz，故障控制 /control/*）"
echo "主服务:   http://127.0.0.1:8080  （账号 admin/admin123、buyer/buyer123）"
if [ "${1:-}" = "dev" ]; then
  echo "==> 启动 Vite 开发服务器 :5173"
  (cd "$ROOT/frontend" && "$NPM" run dev)
fi
