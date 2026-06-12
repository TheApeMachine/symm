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

.PHONY: build test test-go test-race test-cover test-e2e test-frontend bench run audit audit-report dump profile profile-stack profile-report strip-trailing-newlines

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
	cd frontend && pnpm exec tsc --noEmit -p tsconfig.lib.json && pnpm test --run

bench:
	go test $(LDFLAGS) -bench=. -benchmem ./...

run:
	@echo "symm running (Ctrl+C to stop)"
	@echo "UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	go run $(LDFLAGS) main.go

dump:
	python3 scripts/dump-repo.py $(DUMP_OUTPUT)

strip-trailing-newlines:
	git ls-files '*.go' | python3 scripts/strip-trailing-newlines.py

