# DMT currently pulls qpool's go:linkname runtime hooks for cognitive code.
# Go 1.26+ needs this while that indirect dependency remains.
# export GOFLAGS so make targets and nested go/cgo subprocesses inherit the flag.
# Outside Make, run: export GOFLAGS=-ldflags=-checklinkname=0
# No inner quotes: a single shell layer passes the flag through unambiguously.
export GOFLAGS := -ldflags=-checklinkname=0
export GOEXPERIMENT := arenas

LDFLAGS := $(GOFLAGS)

# Metal pipeline compilation is XPC-global, so package binaries must not
# initialize independent domains concurrently. The full-replay packages drive
# the real Metal solver per frame and legitimately need more than Go's 600s
# default per-binary budget.
GO_TEST_FLAGS := -p 1 -timeout 30m

SYMM_BIN := bin/symm
# Leave CONFIG empty to use the binary's default configuration search.
# Set CONFIG=path to select an explicit config file.
CONFIG ?=
CONFIG_FLAG = $(if $(CONFIG),--config $(CONFIG),)
LOG_DIR ?= runs
ADVISOR_CONFIG ?= $(if $(wildcard $(HOME)/.symm/data/advisors.json),$(HOME)/.symm/data/advisors.json,config/advisors.json)
ADVISOR_OUT ?= runs/advisors.candidate.json
ADVISOR_RUN ?=
ADVISOR_FLAGS ?=

DUMP_OUTPUT ?= symm.txt

.PHONY: build test test-go test-race test-cover test-e2e test-frontend bench run collect detect audit workbench dump profile profile-stack profile-report strip-trailing-newlines debug debug-inspect backtest generate-telemetry physics-metallib physics-manifold-metallib experimental metric-lineage metric-map build-cuda

generate-telemetry:
	flatc --no-warnings --go --gen-object-api -o telemetry/generated telemetry/telemetry.fbs
	flatc --no-warnings --ts --gen-object-api -o frontend/src/providers/telemetry telemetry/telemetry.fbs
	find frontend/src/providers/telemetry -type f -name '*.ts' -print0 | xargs -0 perl -pi -e 'if ($$. == 1 && $$_ ne "// \@ts-nocheck\n") { print "// \@ts-nocheck\n" } close ARGV if eof'

test: test-go test-race test-frontend

test-go:
	go test $(LDFLAGS) $(GO_TEST_FLAGS) ./...

test-race:
	go test $(LDFLAGS) $(GO_TEST_FLAGS) -race ./...

test-cover:
	@mkdir -p runs
	go test $(LDFLAGS) $(GO_TEST_FLAGS) -coverprofile=runs/coverage.out ./...
	go tool cover -func=runs/coverage.out | tail -1

test-frontend:
	cd frontend && pnpm build

bench:
	go test $(LDFLAGS) $(GO_TEST_FLAGS) -bench=. -benchmem ./...

kill:
	-lsof -t -i:8765 | xargs kill -9 || true

metric-lineage:
	go run ./tools/metriclineage . frontend/public/metric-lineage.json

metric-map:
	go run ./tools/metricmap signal/metric_map.csv signal/metric_map.json

# metric-lineage is a separate audit target. Regenerating it on every `make run`
# currently collapses producers (Number/SetMetric pipelines are invisible to the
# static Project/Binding scanner) and the Influence UI loses most of the graph.
run:
	go run main.go

collect:
	@echo "symm collect running (Ctrl+C to stop)"
	go run $(LDFLAGS) main.go collect $(CONFIG_FLAG)

# The fee must be the one detect ran with (TAKER_FEE_PERCENT, same as detect
# --taker-fee-percent); the audit checks it against the stored detections.
# Override: make audit TAKER_FEE_PERCENT=0.26 ARGS="--epoch 1791596128450467000"
TAKER_FEE_PERCENT ?= 0.8
MIN_MOVE_DURATION ?= 1s

detect:
	@echo "Labeling spot:trade tape with excursion detections (taker fee $(TAKER_FEE_PERCENT)%, min move $(MIN_MOVE_DURATION))..."
	go run $(LDFLAGS) main.go detect $(CONFIG_FLAG) --taker-fee-percent $(TAKER_FEE_PERCENT) --min-move-duration $(MIN_MOVE_DURATION) $(ARGS)

audit:
	@echo "Running SYMM sensory & representation health audit (taker fee $(TAKER_FEE_PERCENT)%)..."
	go run $(LDFLAGS) main.go audit $(CONFIG_FLAG) --ticks 40000 --taker-fee-percent $(TAKER_FEE_PERCENT) $(ARGS)

# Analytical Workbench is a separate process on purpose: DuckDB/cgo must not share
# the trading binary. Hub proxies POST /workbench/query → workbench.url (default
# http://127.0.0.1:8081/workbench/query). Start this beside `make run` / `pnpm dev`
# for /workbench and Hindsight research timelines; trading stays up if it dies.
workbench:
	@echo "symm-workbench on :8081 (Ctrl+C to stop) — hub proxies /workbench/query here"
	go run $(LDFLAGS) ./cmd/workbench

experimental:
	@echo "symm running (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws · fluid http://127.0.0.1:8765/webrtc/manifold — dashboard: cd frontend && pnpm dev"
	@echo "Analytical workbench (separate): make workbench"
	go run $(LDFLAGS) main.go experimental

backtest:
	@echo "symm running (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws · fluid http://127.0.0.1:8765/webrtc/manifold — dashboard: cd frontend && pnpm dev"
	go run $(LDFLAGS) main.go backtest

debug:
	@echo "symm debug running (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws · fluid http://127.0.0.1:8765/webrtc/manifold — dashboard: cd frontend && pnpm dev"
	export DATURA_INSPECT=1 && go run $(LDFLAGS) main.go

debug-inspect:
	@echo "symm debug (DATURA_INSPECT) running (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws · fluid http://127.0.0.1:8765/webrtc/manifold — dashboard: cd frontend && pnpm dev"
	export DATURA_INSPECT=1 && go run $(LDFLAGS) main.go

run-profile:
	@echo "pprof http://127.0.0.1:6060/debug/pprof/"
	SYMM_PPROF=1 go run $(LDFLAGS) main.go

profile:
	curl -o profile http://127.0.0.1:6060/debug/pprof/profile?seconds=30

profile-report:
	go tool pprof -top profile

dump:
	python3 scripts/dump-repo.py $(DUMP_OUTPUT)
	split -n 2 symm.txt
	mv xaa symm1.txt
	mv xab symm2.txt

strip-trailing-newlines:
	git ls-files '*.go' | python3 scripts/strip-trailing-newlines.py

physics-metallib: physics-manifold-metallib

physics-manifold-metallib:
	cd nomagique/physics/sensorium && go run ./metallibgen

build: physics-metallib
	@mkdir -p bin
	go build $(LDFLAGS) -race -o $(SYMM_BIN) .

build-cuda:
	p=nomagique/physics/sensorium
	cmake -S "$p/cuda" -B "$p/cuda/build" \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_CUDA_ARCHITECTURES=native
	cmake --build "$p/cuda/build" --parallel
	ctest --test-dir "$p/cuda/build" --output-on-failure
	go test -tags cuda ./nomagique/physics/sensorium
