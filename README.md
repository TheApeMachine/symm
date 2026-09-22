![Header image of S.Y.M.M.](symm.png)

# S.Y.M.M. — Shake Your Money Maker

SYMM is a personal market-system research project. The current application compiles
Flume JSON graphs into executable Cap’n Proto programs. Graph nodes are capabilities;
their schemas define input and output ports, and the compiler resolves connections
before execution.

The architecture contract is [ARCHITECTURE.md](ARCHITECTURE.md). The current entry
point is [cmd/root.go](cmd/root.go), and the default graph is
[manifest/system.json](manifest/system.json). Earlier advisor, broker and learning
pipelines described here have been removed; they are not the current application.

This is experimental software, with no guarantee of profitability or fitness for
trading real funds.

## Current execution path

1. The CLI reads `--graph` (default: `manifest/system.json`).
2. The compiler resolves definitions and constructors, reflects Cap’n Proto
   interfaces, validates connections and authored values, and builds a `Program`.
3. `Program.Start` repeatedly evaluates the graph until its context is cancelled.
4. External WebSocket clients supply received frames. Idle backoff distinguishes
   those observations from values circulating within the graph.
5. The workbench exposes graph compilation and individual execution requests to
   the frontend. The editor and dynamic UI show backend results associated with
   the graph revision that produced them.

The default manifest includes Kraken connection and ping nodes. A connection or
successful ping is not evidence of market subscriptions, learning, order execution,
or dashboard delivery; those paths require their own graph and runtime verification.

## Layout

- `cmd/`: CLI and graph startup.
- `manifest/`: graph definitions embedded by the definition repository.
- `nomagique/compiler/`: schema reflection, graph compilation, evaluation and workbench.
- `nomagique/runtime/`: lifecycle, source protocol and streaming workspace.
- `nomagique/`: numerical primitives, data operations, storage and transports.
- `frontend/`: graph editor, compiled UI renderer and inspection views.

## Build and run

The Go version is declared in `go.mod`. The Makefile supplies the linker setting
required by the current indirect dependencies.

```sh
make build
make run
```

To select a graph directly:

```sh
GOFLAGS=-ldflags=-checklinkname=0 go run . --graph manifest/system.json
```

For the frontend, from `frontend/`:

```sh
pnpm install
pnpm dev
```

## Verification

Focused backend verification:

```sh
GOFLAGS=-ldflags=-checklinkname=0 go test -race -p 1 ./cmd ./nomagique/compiler ./nomagique/temporal ./nomagique/runtime/... ./nomagique/network/...
```

Frontend verification, from `frontend/`:

```sh
pnpm test
pnpm typecheck
pnpm build
```

`make test-go` runs the full Go suite; `make bench` runs Go benchmarks. These may
exercise platform-specific numerical implementations. Passing isolated tests does
not establish sustained live operation.

When regenerating Cap’n Proto bindings, compile all schemas in each affected
package together: the Go generator emits one schema registration function per
package. Generating individual files separately can create duplicate registrations.
