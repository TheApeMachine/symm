# qpool uses go:linkname runtime hooks; Go 1.26+ needs this when linking symm.
LDFLAGS := -ldflags='-checklinkname=0'

SYMM_BIN := bin/symm
LOG_DIR ?= runs
UI_ADDR ?= :8765

.PHONY: build test bench run

build:
	@mkdir -p $(LOG_DIR)
	go build $(LDFLAGS) -o $(SYMM_BIN) .

test:
	go test $(LDFLAGS) ./...

bench:
	go test $(LDFLAGS) -bench=. -benchmem ./...

run: build
	@echo "symm running (Ctrl+C to stop). UI ws://127.0.0.1$(UI_ADDR)/ws — dashboard: cd frontend && pnpm dev"
	@echo "Dry-run replay: make replay REPLAY_FILE=replay/fixtures/sample.jsonl"
	./$(SYMM_BIN) \
		--log-file-active \
		--log-dir $(LOG_DIR) \
		--log-stdout \
		--ui-addr $(UI_ADDR)

REPLAY_FILE ?=
REPLAY_PACE ?= 50ms

.PHONY: replay

replay: build
	@test -n "$(REPLAY_FILE)" || (echo "REPLAY_FILE is required, e.g. make replay REPLAY_FILE=replay/fixtures/sample.jsonl" && exit 1)
	./$(SYMM_BIN) \
		--replay-file $(REPLAY_FILE) \
		--replay-pace $(REPLAY_PACE) \
		--log-file-active \
		--log-dir $(LOG_DIR) \
		--log-stdout \
		--ui-addr $(UI_ADDR)

# Long paper session (file only, debug near-misses):
# ./bin/symm --log-level debug --log-dir runs --ui-addr :8765
