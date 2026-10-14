#!/usr/bin/env bash
set -e

# If workbench is already healthy on :8081, just run the command directly.
if curl -s http://127.0.0.1:8081/health >/dev/null 2>&1; then
    echo "[workbench] DuckDB server already running on :8081"
    exec "$@"
fi

echo "[workbench] Starting analytical workbench (DuckDB) on :8081..."
go run ./cmd/workbench &
WB_PID=$!

cleanup() {
    echo ""
    echo "[workbench] Stopping analytical workbench (PID $WB_PID)..."
    kill -TERM "$WB_PID" 2>/dev/null || true
    for _ in $(seq 1 10); do
        if ! kill -0 "$WB_PID" 2>/dev/null; then
            break
        fi
        sleep 0.2
    done
    lsof -t -i:8081 | xargs kill -9 2>/dev/null || true
}

trap cleanup EXIT INT TERM

READY=false
for i in $(seq 1 40); do
    if curl -s http://127.0.0.1:8081/health >/dev/null 2>&1; then
        READY=true
        echo "[workbench] DuckDB server ready on :8081"
        break
    fi
    sleep 0.1
done

if [ "$READY" != "true" ]; then
    echo "[workbench] Warning: DuckDB server did not become healthy within 4s, continuing anyway..."
fi

"$@"
