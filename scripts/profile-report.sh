#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PROFILE_DIR="${PROFILE_DIR:-$ROOT/runs/profiles}"
LDFLAGS="-ldflags=-checklinkname=0"

mkdir -p "$PROFILE_DIR"

echo "=== symm profile report ==="
echo "output: $PROFILE_DIR"
echo

run_bench_profile() {
  local name="$1"
  shift
  local cpu="$PROFILE_DIR/${name}-cpu.prof"
  local mem="$PROFILE_DIR/${name}-mem.prof"

  echo "--- collecting $name ---"
  go test $LDFLAGS \
    -cpuprofile="$cpu" \
    -memprofile="$mem" \
    -benchmem \
    "$@" || return 1

  echo
  echo "[$name cpu top]"
  go tool pprof -top -nodecount=20 "$cpu" 2>/dev/null | sed -n '1,25p'
  echo
  echo "[$name cpu top cum]"
  go tool pprof -top -cum -nodecount=20 "$cpu" 2>/dev/null | sed -n '1,25p'
  echo
  echo "[$name mem top]"
  go tool pprof -top -alloc_space -nodecount=20 "$mem" 2>/dev/null | sed -n '1,25p'
  echo
}

cd "$ROOT"

run_bench_profile stack \
  -bench=BenchmarkProfileStack \
  -benchtime=15s \
  ./profile/...

run_bench_profile hotpath \
  -bench='Benchmark(Bivariate|LeadLag|DepthFlow|Hayashi|CryptoEngine|Fluid|ParseTop|ParseTrades|Instrument)' \
  -benchtime=5s \
  ./hawkes/... ./leadlag/... ./depthflow/... ./correlation/... ./trader/... ./fluid/... ./kraken/market/...

echo "profiles written under $PROFILE_DIR"
echo "inspect: go tool pprof -http=:0 $PROFILE_DIR/stack-cpu.prof"
