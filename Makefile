# qpool uses go:linkname runtime hooks; Go 1.26+ needs this when linking symm.
# export GOFLAGS so make targets and nested go/cgo subprocesses inherit the flag.
# Outside Make, run: export GOFLAGS=-ldflags=-checklinkname=0
# No inner quotes: a single shell layer passes the flag through unambiguously.
export GOFLAGS := -ldflags=-checklinkname=0

LDFLAGS := $(GOFLAGS)

SYMM_BIN := bin/symm
# Leave CONFIG empty to use the binary's own default loading (cmd/cfg/infra.yml +
# strategy.yml — the documented source of truth). Set CONFIG=path to override.
# The previous default silently loaded the legacy merged config.yml instead.
CONFIG ?=
CONFIG_FLAG = $(if $(CONFIG),--config $(CONFIG),)
LOG_DIR ?= runs
# Tune knobs — these are now actually passed to the binary; they were previously
# defined here and never used, so beam width silently ran at the code default.
TUNE_WORKERS ?= 0
TUNE_BEAM_WIDTH ?= 0
TUNE_MAX_ROUNDS ?= 0
TUNE_MAX_NODES ?= 0
TUNE_PATIENCE ?= 0
TUNE_MAX_MEASUREMENTS ?= 0
TUNE_FLAGS = --workers $(TUNE_WORKERS) --beam-width $(TUNE_BEAM_WIDTH) --max-rounds $(TUNE_MAX_ROUNDS) --max-nodes $(TUNE_MAX_NODES) --patience $(TUNE_PATIENCE) --max-measurements $(TUNE_MAX_MEASUREMENTS)

DUMP_OUTPUT ?= symm.txt

.PHONY: build physics-metallib test test-go test-race test-cover test-e2e test-frontend bench run audit audit-report tune replay dump profile profile-stack profile-report profile-tune strip-trailing-newlines

physics-metallib:
	cd numeric/physics && go run ./metallibgen

build: physics-metallib
	@mkdir -p $(LOG_DIR)
	go build -o $(SYMM_BIN) .

test: test-go test-race test-frontend

test-go: physics-metallib
	go test ./...

test-e2e:
	@mkdir -p runs
	go test $(LDFLAGS) ./integration/... -count=1 -timeout 360s

test-race:
	go test -race ./...

test-cover:
	@mkdir -p runs
	go test $(LDFLAGS) -coverprofile=runs/coverage.out ./...
	go tool cover -func=runs/coverage.out | tail -1

test-frontend:
	cd frontend && pnpm exec tsc --noEmit -p tsconfig.lib.json && pnpm test --run

bench: physics-metallib
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
	SYMM_PPROF=1 ./$(SYMM_BIN) tune $(CONFIG_FLAG) $(TUNE_FLAGS)

run: build
	@mkdir -p $(LOG_DIR)
	@echo "symm running — collecting run data → runs/capture.jsonl  (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	./$(SYMM_BIN) $(CONFIG_FLAG) --record

audit: build
	@mkdir -p $(LOG_DIR)
	@echo "symm running with desk audit log (trading.audit.file in $(CONFIG))"
	@echo "  gate_reject deduped (60s), rotates at 32MB × 3 files"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	./$(SYMM_BIN) $(CONFIG_FLAG)

audit-report:
	@test -f $(LOG_DIR)/audit.jsonl || (echo "No audit log yet — run 'make audit' or 'make run' first." && exit 1)
	go run ./scripts/auditreport $(LOG_DIR)/audit.jsonl

run-profile: build
	@echo "symm running (Ctrl+C to stop). UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	SYMM_PPROF=1 ./$(SYMM_BIN) $(CONFIG_FLAG)

tune: build
	@test -f runs/capture.jsonl || (echo "No run data yet — do 'make run' to collect it (Ctrl+C when you have enough), then 'make tune'." && exit 1)
	./$(SYMM_BIN) tune $(CONFIG_FLAG) $(TUNE_FLAGS)

# Manual workbench: score a hand-written playbook against the capture, per-setup.
#   make replay                              (live playbook vs default capture)
#   make replay PLAYBOOK=my-ideas.yaml       (your experiment)
PLAYBOOK ?=
replay: build
	@test -f runs/capture.jsonl || (echo "No run data yet — do 'make run' to collect it first." && exit 1)
	./$(SYMM_BIN) replay $(CONFIG_FLAG) $(if $(PLAYBOOK),--playbook $(PLAYBOOK),)

dump:
	python3 scripts/dump-repo.py $(DUMP_OUTPUT)

strip-trailing-newlines:
	git ls-files '*.go' | python3 scripts/strip-trailing-newlines.py

