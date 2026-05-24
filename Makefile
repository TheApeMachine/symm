# qpool uses go:linkname runtime hooks; Go 1.26+ needs this when linking symm.
LDFLAGS := -ldflags='-checklinkname=0'

SYMM_BIN := bin/symm
LOG_DIR ?= runs

.PHONY: build test test-go test-race test-frontend bench run replay eval

build:
	@mkdir -p $(LOG_DIR)
	go build $(LDFLAGS) -o $(SYMM_BIN) .

test: test-go test-frontend

test-go:
	go test $(LDFLAGS) ./...

test-race:
	go test $(LDFLAGS) -race ./...

test-frontend:
	cd frontend && pnpm test

bench:
	go test $(LDFLAGS) -bench=. -benchmem ./...

run: build
	@echo "symm running (Ctrl+C to stop). UI ws://127.0.0.1:8765/ws — dashboard: cd frontend && pnpm dev"
	@echo "Dry-run replay: make replay REPLAY_FILE=replay/fixtures/sample.jsonl"
	./$(SYMM_BIN)

REPLAY_FILE ?=
REPLAY_PACE ?= 50ms

replay: build
	@test -n "$(REPLAY_FILE)" || (echo "REPLAY_FILE is required, e.g. make replay REPLAY_FILE=replay/fixtures/sample.jsonl" && exit 1)
	SYMM_REPLAY_FILE=$(REPLAY_FILE) SYMM_REPLAY_PACE=$(REPLAY_PACE) ./$(SYMM_BIN)

eval:
	@test -n "$(REPLAY_FILE)" || (echo "REPLAY_FILE is required, e.g. make eval REPLAY_FILE=replay/fixtures/sample.jsonl" && exit 1)
	go run $(LDFLAGS) . eval --file $(REPLAY_FILE) --format $(or $(FORMAT),json)
