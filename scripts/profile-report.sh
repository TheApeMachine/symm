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
  local package="$2"
  local bench="$3"
  local benchtime="$4"
  local cpu="$PROFILE_DIR/${name}-cpu.prof"
  local mem="$PROFILE_DIR/${name}-mem.prof"

  echo "--- collecting $name ---"
  go test $LDFLAGS \
    -cpuprofile="$cpu" \
    -memprofile="$mem" \
    -benchmem \
    -run='^$' \
    -bench="$bench" \
    -benchtime="$benchtime" \
    "$package" || return 1

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

run_bench_profile optimizer-reasoning \
  ./optimizer/reasoning \
  'Benchmark(Search|Neighbors|TemporalizeEntry|KeyOf|CloneForest)' \
  10s

run_bench_profile optimizer-replay \
  ./optimizer/replay \
  'Benchmark(PrecompileTape|ThoughtSimulationResult|CheckTriggers|ReplayLedger|ReplayMeasurements)' \
  5s

run_bench_profile market-perspectives \
  ./market/perspectives \
  'Benchmark(ClassifyRegime|WindowReasonReset|EvaluateStateful|CategoriesClassify)' \
  5s

run_bench_profile signal-hawkes \
  ./signal/hawkes \
  'Benchmark(ObserveTrades|Measure|Bivariate)' \
  5s

run_bench_profile signal-leadlag \
  ./signal/leadlag \
  'BenchmarkMeasureFollower' \
  5s

run_bench_profile signal-depthflow \
  ./signal/depthflow \
  'Benchmark(ObserveTrade|DepthSymbolMeasure)' \
  5s

run_bench_profile signal-fluid \
  ./signal/fluid \
  'Benchmark(PublishField|Emit|FluidSymbolMeasure|FluxAccumulatorAddTrade)' \
  5s

run_bench_profile broker \
  ./broker \
  'Benchmark(QuoteCacheSnapshot|StressCache|PreflightGates|SlippageFill|Maker)' \
  5s

run_bench_profile kraken-market \
  ./kraken/market \
  'Benchmark(BookFold|Instrument|Parse|Trade|Ticker)' \
  5s

echo "profiles written under $PROFILE_DIR"
echo "inspect: go tool pprof -http=:0 $PROFILE_DIR/optimizer-reasoning-cpu.prof"
