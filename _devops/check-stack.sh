#!/usr/bin/env bash
# Throwaway stack to check a branch end to end: a fresh Postgres in Docker and
# the API built from this checkout (its migrations included), served under
# /api, with the seed account loaded. Runs beside `docker compose up`, which
# keeps serving main on :8080.
#
#   _devops/check-stack.sh up [PORT]     # default PORT 8081
#   _devops/check-stack.sh down [PORT]
#
# The front points its dev proxy at it with API_PROXY=http://localhost:PORT.
set -euo pipefail

cmd=${1:-}
port=${2:-8081}
root=$(cd "$(dirname "$0")/.." && pwd)
state="${TMPDIR:-/tmp}/tamiyo-check-$port"
container="tamiyo-check-db-$port"

down() {
    if [ -f "$state/api.pid" ]; then
        kill "$(cat "$state/api.pid")" 2>/dev/null || true
    fi
    docker rm -f "$container" >/dev/null 2>&1 || true
    rm -rf "$state"
    echo "check-stack: stopped (port $port)"
}

up() {
    if [ -e "$state" ]; then
        echo "check-stack: already up on port $port (run: $0 down $port)" >&2
        exit 1
    fi
    mkdir -p "$state"

    docker run -d --rm --name "$container" \
        -e POSTGRES_USER=login -e POSTGRES_PASSWORD=password -e POSTGRES_DB=tamiyo_db \
        -p 127.0.0.1::5432 postgres:16 >/dev/null
    db_port=$(docker port "$container" 5432/tcp | head -1 | sed 's/.*://')

    (cd "$root" && go build -o "$state/tamiyo-api" ./cmd/api)

    # Over TCP: the image's init server answers on its Unix socket, then restarts.
    for _ in $(seq 1 30); do
        docker exec "$container" pg_isready -h 127.0.0.1 -U login -d tamiyo_db >/dev/null 2>&1 && break
        sleep 1
    done

    PGHOST=127.0.0.1 PGPORT="$db_port" PGUSER=login PGPASSWORD=password PGDATABASE=tamiyo_db PGSSLMODE=disable \
        APP_PORT="$port" API_BASE_PATH=/api JWT_SECRET=check-stack-secret-check-stack-secret \
        REFRESH_COOKIE_SECURE=false AUTH_RATE_LIMIT_MAX=1000 \
        nohup "$state/tamiyo-api" >"$state/api.log" 2>&1 &
    echo $! >"$state/api.pid"

    for _ in $(seq 1 30); do
        curl -sf "localhost:$port/health" >/dev/null && break
        sleep 1
    done
    if ! curl -sf "localhost:$port/health" >/dev/null; then
        echo "check-stack: API did not start, see $state/api.log" >&2
        tail -5 "$state/api.log" >&2
        down
        exit 1
    fi

    docker exec -i -e PGOPTIONS='-c client_min_messages=warning' "$container" psql -q -U login -d tamiyo_db <"$root/_devops/database/seedTestData.sql" >/dev/null

    echo "check-stack: API http://localhost:$port/api (log: $state/api.log)"
    echo "check-stack: seed account seed@tamiyo.local / password123"
    echo "check-stack: front: API_PROXY=http://localhost:$port npx vite --port 5174 --strictPort"
}

case "$cmd" in
up) up ;;
down) down ;;
*)
    echo "usage: $0 up|down [PORT]" >&2
    exit 2
    ;;
esac
