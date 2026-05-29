# qpool uses go:linkname runtime hooks; Go 1.26+ needs this when linking symm.
# Always use make test-go / make build — bare `go test ./...` fails at link time.
# No inner quotes: a single shell layer can pass the flag through unambiguously,
# but quoted forms break in nested shells (cgo, subprocesses).
LDFLAGS := -ldflags=-checklinkname=0

SYMM_BIN := bin/symm
LOG_DIR ?= runs

RACE_PACKAGES := $(shell go list ./... | grep -v '/engine$$')

DUMP_OUTPUT ?= symm.txt

.PHONY: build test test-go test-race test-frontend bench run replay dump profile profile-stack profile-report

build:
	@mkdir -p $(LOG_DIR)
	go build $(LDFLAGS) -o $(SYMM_BIN) .

test: test-go test-race test-frontend

test-go:
	go test $(LDFLAGS) -race ./...

test-race:
ifeq ($(shell uname -s),Darwin)
	go test $(LDFLAGS) -race $(RACE_PACKAGES)
else
	go test $(LDFLAGS) -race ./...
endif

test-frontend:
	cd frontend && pnpm test

bench:
	go test $(LDFLAGS) -bench=. -benchmem ./...

PROFILE_DIR ?= runs/profiles

profile:
	@mkdir -p $(PROFILE_DIR)
	go test $(LDFLAGS) -cpuprofile=$(PROFILE_DIR)/bench-cpu.prof -memprofile=$(PROFILE_DIR)/bench-mem.prof -bench=. ./...

profile-stack:
	@mkdir -p $(PROFILE_DIR)
	go test $(LDFLAGS) \
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

run-profile: build
	@echo "symm running (Ctrl+C to stop). UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	@echo "Replay: make replay REPLAY_FILE=replay/fixtures/sample.jsonl"
	SYMM_PPROF=1 ./$(SYMM_BIN)

REPLAY_FILE ?=
REPLAY_PACE ?= 50ms

replay: build
	@test -n "$(REPLAY_FILE)" || (echo "REPLAY_FILE is required, e.g. make replay REPLAY_FILE=replay/fixtures/sample.jsonl" && exit 1)
	SYMM_REPLAY_FILE=$(REPLAY_FILE) SYMM_REPLAY_PACE=$(REPLAY_PACE) ./$(SYMM_BIN)

dump:
	python3 scripts/dump-repo.py $(DUMP_OUTPUT)
