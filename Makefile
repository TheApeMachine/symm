# DMT currently pulls qpool's go:linkname runtime hooks for cognitive code.
# Go 1.26+ needs this while that indirect dependency remains.
# export GOFLAGS so make targets and nested go/cgo subprocesses inherit the flag.
# Outside Make, run: export GOFLAGS=-ldflags=-checklinkname=0
# No inner quotes: a single shell layer passes the flag through unambiguously.
export GOEXPERIMENT := arenas

# Metal pipeline compilation is XPC-global, so package binaries must not
# initialize independent domains concurrently. The full-replay packages drive
# the real Metal solver per frame and legitimately need more than Go's 600s
# default per-binary budget.
OVERLAY_FLAG := $(if $(wildcard overlay.json),-overlay=$(CURDIR)/overlay.json,)
GO_TEST_FLAGS := -p 1 -timeout 30m

SYMM_BIN := bin/symm
# Leave CONFIG empty to use the binary's default configuration search.
# Set CONFIG=path to select an explicit config file.
CONFIG ?=
CONFIG_FLAG = $(if $(CONFIG),--config $(CONFIG),)
LOG_DIR ?= runs
ADVISOR_CONFIG ?= $(if $(wildcard $(HOME)/.symm/data/advisors.json),$(HOME)/.symm/data/advisors.json,config/advisors.json)
ADVISOR_OUT ?= runs/advisors.candidate.json
ADVISOR_RUN ?=
ADVISOR_FLAGS ?=

DUMP_OUTPUT ?= symm.txt

.PHONY: build test test-flow test-go test-race test-cover test-e2e test-frontend bench run paper dump strip-trailing-newlines physics-metallib physics-manifold-metallib build-cuda

test: test-flow test-go test-race test-frontend

test-flow:
	go run ./tools/controlflow .

test-go:
	go test $(GO_TEST_FLAGS) ./...

test-race:
	go test $(GO_TEST_FLAGS) -race ./...

test-cover:
	@mkdir -p runs
	go test $(GO_TEST_FLAGS) -coverprofile=runs/coverage.out ./...
	go tool cover -func=runs/coverage.out | tail -1

test-frontend:
	cd frontend && pnpm build

bench:
	go test $(GO_TEST_FLAGS) -bench=. -benchmem ./...

# metric-lineage is a separate audit target. Regenerating it on every `make run`
# currently collapses producers (Number/SetMetric pipelines are invisible to the
# static Project/Binding scanner) and the Influence UI loses most of the graph.
run:
	go run main.go $(CONFIG_FLAG)

# paper uploads the records `make run` appended to paper.records_path; run
# itself never writes storage.
paper:
	go run main.go paper $(CONFIG_FLAG)

# The fee must be the one detect ran with (TAKER_FEE_PERCENT, same as detect
# --taker-fee-percent); the audit checks it against the stored detections.
# Override: make audit TAKER_FEE_PERCENT=0.26 ARGS="--epoch 1791596128450467000"
TAKER_FEE_PERCENT ?= 0.8
MIN_MOVE_DURATION ?= 1s

dump:
	python3 scripts/dump-repo.py $(DUMP_OUTPUT)
	split -n 2 symm.txt
	mv xaa symm1.txt
	mv xab symm2.txt

strip-trailing-newlines:
	git ls-files '*.go' | python3 scripts/strip-trailing-newlines.py

physics-metallib: physics-manifold-metallib

physics-manifold-metallib:
	cd nomagique/physics/sensorium && go run ./metallibgen

build: physics-metallib
	@mkdir -p bin
	go build -race -o $(SYMM_BIN) .

build-cuda:
	p=nomagique/physics/sensorium
	cmake -S "$p/cuda" -B "$p/cuda/build" \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_CUDA_ARCHITECTURES=native
	cmake --build "$p/cuda/build" --parallel
	ctest --test-dir "$p/cuda/build" --output-on-failure
	go test -tags cuda ./nomagique/physics/sensorium
