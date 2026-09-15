#!/usr/bin/env bash
# ============================================================
# ops web 启动脚本
# 用固定登录口令启动后端。口令写在本文件中，权限应为 600。
# 用法:  ./start.sh [端口]        (默认 8090)
# 停止:  kill "$(cat server.pid)"
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ---------- 登录口令 ----------
export OPS_PASSWORD='asd620522-'

PORT="${1:-8090}"

# 若已有实例在跑, 先停掉
if [[ -f server.pid ]]; then
  OLD="$(cat server.pid 2>/dev/null || true)"
  if [[ -n "$OLD" ]] && kill -0 "$OLD" 2>/dev/null; then
    echo "停止旧进程 $OLD"
    kill "$OLD"
    sleep 1
  fi
fi

# 后台启动, 日志追加到 server.log
nohup python3 ./server.py "$PORT" >> server.log 2>&1 &
echo $! > server.pid
sleep 1
echo "ops web 已启动: http://0.0.0.0:$PORT  (pid $(cat server.pid))"
