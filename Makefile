# qpool uses go:linkname runtime hooks; Go 1.26+ needs this when linking symm.
# export GOFLAGS so make targets and nested go/cgo subprocesses inherit the flag.
# Outside Make, run: export GOFLAGS=-ldflags=-checklinkname=0
# No inner quotes: a single shell layer passes the flag through unambiguously.
export GOFLAGS := -ldflags=-checklinkname=0

LDFLAGS := $(GOFLAGS)

SYMM_BIN := bin/symm
LOG_DIR ?= runs

RACE_PACKAGES := $(shell go list ./... | grep -v '/engine$$')

DUMP_OUTPUT ?= symm.txt

.PHONY: build test test-go test-race test-cover test-frontend bench run audit replay record tune dump profile profile-stack profile-report strip-trailing-newlines

build:
	@mkdir -p $(LOG_DIR)
	go build -o $(SYMM_BIN) .

test: test-go test-race test-frontend

test-go:
	go test -race ./...

test-race:
ifeq ($(shell uname -s),Darwin)
	go test -race $(RACE_PACKAGES)
else
	go test -race ./...
endif

test-cover:
	@mkdir -p runs
	go test -coverprofile=runs/coverage.out ./...
	go tool cover -func=runs/coverage.out | tail -1

test-frontend:
	cd frontend && pnpm exec tsc --noEmit -p tsconfig.lib.json && pnpm test --run

bench:
	go test -bench=. -benchmem ./...

PROFILE_DIR ?= runs/profiles

profile:
	@mkdir -p $(PROFILE_DIR)
	go test -cpuprofile=$(PROFILE_DIR)/bench-cpu.prof -memprofile=$(PROFILE_DIR)/bench-mem.prof -bench=. ./...

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

profile-replay: build
	@mkdir -p $(PROFILE_DIR)
	@test -n "$(REPLAY_FILE)" || (echo "REPLAY_FILE is required" && exit 1)
	@echo "Starting replay with pprof on :6060 — capture with:"
	@echo "  curl -o $(PROFILE_DIR)/replay-cpu.prof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'"
	SYMM_PPROF=1 SYMM_REPLAY_LOOP=1 SYMM_LOG_STDOUT=1 \
		SYMM_REPLAY_FILE=$(REPLAY_FILE) ./$(SYMM_BIN)

run: build
	@echo "symm running (Ctrl+C to stop). UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	@echo "Replay: make replay REPLAY_FILE=replay/fixtures/sample.jsonl"
	./$(SYMM_BIN)

AUDIT_FILE ?= $(LOG_DIR)/audit-$(shell date -u +%Y%m%dT%H%M%SZ).jsonl

audit: build
	@mkdir -p $(LOG_DIR)
	@echo "symm running with desk audit log at $(AUDIT_FILE)"
	@echo "  gate_reject deduped (60s), rotates at 32MB × 3 files"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	SYMM_AUDIT_FILE=$(AUDIT_FILE) ./$(SYMM_BIN)

run-profile: build
	@echo "symm running (Ctrl+C to stop). UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	@echo "Replay: make replay REPLAY_FILE=replay/fixtures/sample.jsonl"
	SYMM_PPROF=1 ./$(SYMM_BIN)

REPLAY_PACE ?= 50ms
RECORD_FILE ?= runs/capture.jsonl
REPLAY_FILE ?= $(RECORD_FILE)
TUNE_ITERATIONS ?= 64

replay: build
	@test -f "$(REPLAY_FILE)" || (echo "Missing $(REPLAY_FILE)" && exit 1)
	SYMM_REPLAY_FILE=$(REPLAY_FILE) SYMM_REPLAY_PACE=$(REPLAY_PACE) ./$(SYMM_BIN)

record: build
	@mkdir -p $(dir $(RECORD_FILE)) $(LOG_DIR)
	@echo "Recording live capture to $(RECORD_FILE) (Ctrl+C to stop, then: make tune)"
	@echo "Desk audit log: $(AUDIT_FILE) (for inspection; tune recomputes regret from replay)"
	SYMM_RECORD_FILE=$(RECORD_FILE) SYMM_AUDIT_FILE=$(AUDIT_FILE) ./$(SYMM_BIN)

tune: build
	@test -f "$(REPLAY_FILE)" || (echo "Missing $(REPLAY_FILE). Run: make record" && exit 1)
	@echo "Tuning $(REPLAY_FILE) — fitness = score_eur − missed gate regret ($(TUNE_ITERATIONS) trials)"
	./$(SYMM_BIN) tune --replay "$(REPLAY_FILE)" --iterations $(or $(ITERATIONS),$(TUNE_ITERATIONS)) --workers $(or $(WORKERS),$(shell sysctl -n hw.ncpu 2>/dev/null || nproc))

dump:
	python3 scripts/dump-repo.py $(DUMP_OUTPUT)

strip-trailing-newlines:
	git ls-files '*.go' | python3 scripts/strip-trailing-newlines.py
