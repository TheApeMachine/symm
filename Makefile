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

DUMP_OUTPUT ?= symm.txt

.PHONY: build physics-metallib test test-go test-race test-cover test-e2e test-frontend bench run audit audit-report dump profile profile-stack profile-report strip-trailing-newlines

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

dump:
	python3 scripts/dump-repo.py $(DUMP_OUTPUT)

strip-trailing-newlines:
	git ls-files '*.go' | python3 scripts/strip-trailing-newlines.py

