# qpool uses go:linkname runtime hooks; Go 1.26+ needs this when linking symm.
# export GOFLAGS so make targets and nested go/cgo subprocesses inherit the flag.
# Outside Make, run: export GOFLAGS=-ldflags=-checklinkname=0
# No inner quotes: a single shell layer passes the flag through unambiguously.
export GOFLAGS := -ldflags=-checklinkname=0

LDFLAGS := $(GOFLAGS)

SYMM_BIN := bin/symm
CONFIG ?= cmd/cfg/config.yml
LOG_DIR ?= runs
TUNE_WORKERS ?= $(shell getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu)
TUNE_MAX_THRESHOLDS ?= 128
TUNE_BEAM_WIDTH ?= 256
TUNE_CANDIDATE_LIMIT ?= 2000

RACE_PACKAGES := $(shell go list ./... | grep -v '/engine$$')

DUMP_OUTPUT ?= symm.txt

.PHONY: build test test-go test-race test-cover test-e2e test-frontend bench run audit tune dump profile profile-stack profile-report profile-tune strip-trailing-newlines

build:
	@mkdir -p $(LOG_DIR)
	go build -o $(SYMM_BIN) .

test: test-go test-race test-frontend

test-go:
	go test ./...

test-e2e:
	@mkdir -p runs
	go test $(LDFLAGS) ./integration/... -count=1 -timeout 360s

test-race:
ifeq ($(shell uname -s),Darwin)
	go test -race $(RACE_PACKAGES)
else
	go test -race ./...
endif

test-cover:
	@mkdir -p runs
	go test $(LDFLAGS) -coverprofile=runs/coverage.out ./...
	go tool cover -func=runs/coverage.out | tail -1

test-frontend:
	cd frontend && pnpm exec tsc --noEmit -p tsconfig.lib.json && pnpm test --run

bench:
	go test $(LDFLAGS) -bench=. -benchmem ./...

PROFILE_DIR ?= runs/profiles

profile:
	@mkdir -p $(PROFILE_DIR)
	go test $(LDFLAGS) -cpuprofile=$(PROFILE_DIR)/bench-cpu.prof -memprofile=$(PROFILE_DIR)/bench-mem.prof -bench=. ./...

profile-stack:
	@mkdir -p $(PROFILE_DIR)
	go test \
		-cpuprofile=$(PROFILE_DIR)/stack-cpu.prof \
		-memprofile=$(PROFILE_DIR)/stack-mem.prof \
		-bench=BenchmarkProfileStack \
		-benchtime=15s \
		./profile/...

profile-report:
	@chmod +x scripts/profile-report.sh
	PROFILE_DIR=$(PROFILE_DIR) ./scripts/profile-report.sh

profile-tune: build
	@mkdir -p $(PROFILE_DIR)
	@test -f runs/capture.jsonl || (echo "No run data yet — do 'make run' to collect it, then 'make tune'." && exit 1)
	@echo "=== profile tune ==="
	@echo "Live pprof index: http://127.0.0.1:6060/debug/pprof/"
	SYMM_PPROF=1 ./$(SYMM_BIN) tune --config $(CONFIG)

run: build
	@mkdir -p $(LOG_DIR)
	@echo "symm running — collecting run data → runs/capture.jsonl  (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	./$(SYMM_BIN) --config $(CONFIG) --record

audit: build
	@mkdir -p $(LOG_DIR)
	@echo "symm running with desk audit log (trading.audit.file in $(CONFIG))"
	@echo "  gate_reject deduped (60s), rotates at 32MB × 3 files"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	./$(SYMM_BIN) --config $(CONFIG)

run-profile: build
	@echo "symm running (Ctrl+C to stop). UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	SYMM_PPROF=1 ./$(SYMM_BIN) --config $(CONFIG)

tune: build
	@test -f runs/capture.jsonl || (echo "No run data yet — do 'make run' to collect it (Ctrl+C when you have enough), then 'make tune'." && exit 1)
	./$(SYMM_BIN) tune --config $(CONFIG)

dump:
	python3 scripts/dump-repo.py $(DUMP_OUTPUT)

strip-trailing-newlines:
	git ls-files '*.go' | python3 scripts/strip-trailing-newlines.py

