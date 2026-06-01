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

.PHONY: build test test-go test-race test-cover test-frontend bench run audit replay record tune dump profile profile-stack profile-report profile-tune profile-replay strip-trailing-newlines

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
	@echo "Live pprof index: http://127.0.0.1:6060/debug/pprof/"
	@echo "Capture 30s CPU while replay runs:"
	@echo "  curl -o $(PROFILE_DIR)/replay-cpu.prof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'"
	@echo "Flame graph:"
	@echo "  go tool pprof -http=:0 $(PROFILE_DIR)/replay-cpu.prof"
	SYMM_PPROF=1 ./$(SYMM_BIN) --config cmd/cfg/profile-replay.yml

profile-tune: build
	@mkdir -p $(PROFILE_DIR)
	@test -f "$(REPLAY_FILE)" || (echo "Missing $(REPLAY_FILE). Run: make record" && exit 1)
	@echo "=== profile tune ==="
	@echo "Live pprof index: http://127.0.0.1:6060/debug/pprof/"
	SYMM_PPROF=1 ./$(SYMM_BIN) tune --config cmd/cfg/tune.yml

run: build
	@echo "symm running (Ctrl+C to stop). UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	@echo "Desk config: config/tuned.json + config/perspectives.yaml when present (Go builtins otherwise)"
	@echo "Replay: make replay REPLAY_FILE=runs/capture.jsonl"
	./$(SYMM_BIN) --config cmd/cfg/config.yml

audit: build
	@mkdir -p $(LOG_DIR)
	@echo "symm running with desk audit log (see cmd/cfg/record.yml audit.file)"
	@echo "  gate_reject deduped (60s), rotates at 32MB × 3 files"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	./$(SYMM_BIN) --config cmd/cfg/record.yml

run-profile: build
	@echo "symm running (Ctrl+C to stop). UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	@echo "Replay: make replay REPLAY_FILE=runs/capture.jsonl"
	SYMM_PPROF=1 ./$(SYMM_BIN) --config cmd/cfg/config.yml

REPLAY_PACE ?= 50ms
RECORD_FILE ?= runs/capture.jsonl
REPLAY_FILE ?= $(RECORD_FILE)
TUNED_OUTPUT ?= runs/tuned.json
# Set ITERATIONS=N to cap trials; omit for unlimited until Ctrl+C
PERSPECTIVES_OUTPUT ?= runs/perspectives.yaml
# Robust paper + tuning defaults (override on the make command line)
EXECUTION_STRESS ?= 1
AUTO_HOLDOUT ?= true
WALK_FORWARD_FOLDS ?= 3
REPLAY_PERTURB ?= true
STRESS_HOLDOUT ?= true

replay: build
	@test -f "$(REPLAY_FILE)" || (echo "Missing $(REPLAY_FILE)" && exit 1)
	./$(SYMM_BIN) --config cmd/cfg/replay.yml

record: build
	@mkdir -p $(dir $(RECORD_FILE)) $(LOG_DIR)
	@echo "Recording live capture (Ctrl+C to stop, then: make tune)"
	@echo "Desk config: config/tuned.json + config/perspectives.yaml when present (Go builtins otherwise)"
	@echo "Config: cmd/cfg/record.yml (paper + execution stress + audit log)"
	./$(SYMM_BIN) --config cmd/cfg/record.yml

tune: build
	@test -f "$(REPLAY_FILE)" || (echo "Missing $(REPLAY_FILE). Run: make record" && exit 1)
	@echo "Tuning $(REPLAY_FILE) (Ctrl+C to stop)"
	./$(SYMM_BIN) tune --config cmd/cfg/tune.yml

dump:
	python3 scripts/dump-repo.py $(DUMP_OUTPUT)

strip-trailing-newlines:
	git ls-files '*.go' | python3 scripts/strip-trailing-newlines.py
