# DMT currently pulls qpool's go:linkname runtime hooks for cognitive code.
# Go 1.26+ needs this while that indirect dependency remains.
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
OPTIMIZE_AUDIT ?= runs/audit.jsonl
OPTIMIZE_REPLAY ?= runs/replay.jsonl
OPTIMIZE_TREE ?= logic/rules/tree.yml
OPTIMIZE_LOOKBACK ?= 6h
OPTIMIZE_SYMBOLS ?=
OPTIMIZE_FLAGS ?=

DUMP_OUTPUT ?= symm.txt

.PHONY: build test test-go test-race test-cover test-e2e test-frontend bench run optimize audit audit-report dump profile profile-stack profile-report strip-trailing-newlines debug debug-inspect

test: test-go test-race test-frontend

test-go:
	go test $(LDFLAGS) ./...

test-race:
	go test $(LDFLAGS) -race ./...

test-cover:
	@mkdir -p runs
	go test $(LDFLAGS) -coverprofile=runs/coverage.out ./...
	go tool cover -func=runs/coverage.out | tail -1

test-frontend:
	cd frontend && pnpm build

bench:
	go test $(LDFLAGS) -bench=. -benchmem ./...

kill:
	-lsof -t -i:8765 | xargs kill -9 || true

run:
	@echo "symm running (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	go run $(LDFLAGS) main.go

optimize:
	go run $(LDFLAGS) main.go optimize --replay $(OPTIMIZE_REPLAY) --tree $(OPTIMIZE_TREE) --lookback $(OPTIMIZE_LOOKBACK) --symbols "$(OPTIMIZE_SYMBOLS)" --write-tree $(OPTIMIZE_FLAGS)

debug:
	@echo "symm debug running (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	export DATURA_INSPECT=1 && go run $(LDFLAGS) main.go

debug-inspect:
	@echo "symm debug (DATURA_INSPECT) running (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
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

strip-trailing-newlines:
	git ls-files '*.go' | python3 scripts/strip-trailing-newlines.py

build:
	@mkdir -p bin
	go build $(LDFLAGS) -o $(SYMM_BIN) .
