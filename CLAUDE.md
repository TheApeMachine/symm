# CLAUDE.md

Guidance for Claude Code when working in this repository.

**Read [AGENTS.md](AGENTS.md) before changing code.** It holds the binding rules on ownership, no magic constants, no fallbacks, no NaN/Inf gating, control flow, errors and tests. They are not repeated here, but they apply to every edit.

## What this is

SYMM is a node-graph "visual computer". A Flume JSON graph **is** the program: it is compiled in memory into an executable Cap'n Proto `Program`, and no Go code is generated per graph. The node library (`nomagique/`, roughly 390 node types) is domain-free. The only market-specific area is `nomagique/financial/`. The flagship application is a crypto market system (Kraken) that learns **precursors, not price**:

```
raw capture → virtual grid (metrics) → impulse map → region tokens → radix trie (WAIT/ENTER/EXIT)
```

The market side has no agent, advisors, planner or strategy layer. The trie is the only decision path. Many older docs, memories and `LEGACY.txt` describe packages such as `signal/`, `strategy/`, `trader/`, `logic/`, `broker/` and `types.Value` closures. **Those packages no longer exist.** Check the current tree before trusting any of them.

## Commands

The Makefile exports `GOFLAGS=-ldflags=-checklinkname=0`, which is needed to link because an indirect dependency (qpool) uses `go:linkname`. Outside Make, export it yourself:

```sh
export GOFLAGS=-ldflags=-checklinkname=0

make build                 # rebuilds Metal lib, then go build -race -o bin/symm
make run                   # go run main.go  (defaults to --graph manifest/system.json)
go run . --graph manifest/training.json

make test-go               # go test -p 1 -timeout 30m ./...   (-p 1 is required: Metal is XPC-global)
make test-race
go test -p 1 ./nomagique/compiler -run TestName     # single package / single test
./scripts/verify.sh fmt|test|race|frontend|all      # what CI runs

make catalog               # regenerate node registry + editor config (see below)
make physics-metallib      # after editing any .metal file (kernels are go:embed'ed)
make generate-telemetry    # flatbuffers → Go + TS; never call bare flatc
```

Frontend (`frontend/`, pnpm, TanStack Start + React 19 + Vite, port 3000):

```sh
pnpm install && pnpm dev
pnpm test | pnpm typecheck | pnpm lint | pnpm build
pnpm generate:ui           # reflect React UI library → ui-component-metadata.generated.json
pnpm check:ui-drift        # CI fails if the registry and the UI library disagree
```

`go.mod` has `replace` directives pointing at the sibling checkouts `../nomagique` and `../datura`. CI also checks out `errnie`, `qpool` and `sonic` as siblings. `docker-compose.yml` provides Pyroscope and Grafana for profiling. `cmd/root.go` starts a Pyroscope client pointed at `localhost:4040`.

## Layout

- `main.go`, `cmd/root.go`: Cobra root. It compiles one graph file (`--graph`) with `compiler.CompileFile(..., compiler.DefaultRepository())`, then runs `Start` and `Flush`.
- `manifest/*.json`: the graphs, embedded via `go:embed`. `system.json` is the root. `capture.json` records raw frames. `training*.json` holds archive scan, replay, fragments, grading, pair evidence and reinforcement. `*_ticker|trade|level3.json` are signal sub-graphs. `ui_*.json` are screens. A node of type `definition:<name>` nests another manifest. `compiler.Repository` resolves it from in-memory saves first, then embedded files, then disk.
- `nomagique/compiler/`: schema scanning, the constructor registry (`factories_gen.go`, generated), the compile pipeline, `Program` and runtime, hot recompile and quiescent swap, capability reuse, and the workbench (compile and run single nodes from the editor).
- `nomagique/<area>/`: one node per `foo.capnp` + `foo.capnp.go` (generated) + `foo.go` + `foo_test.go`. The areas are arithmetic, calculus, statistic, probability, distribution, linalg, optimization, sampling, graph, learning, cognition, controlflow, transport, network (websocket/http/webrtc/quic), store (grid, radix, tape, capture, tables/Iceberg), physics (Metal sensorium; CUDA behind `-tags cuda`), ui, financial, and others.
- `workbench/`: server-side DuckDB over the Iceberg catalog, streaming Arrow to a Perspective viewer.
- `frontend/`: Flume node editor (`src/components/flume`), the graph-rendered UI library (`src/components/ui`), and inspection routes (hindsight, learning, lineage, fluid, workbench…).

## Primitive contract (ARCHITECTURE.md is authoritative)

Every node is a Cap'n Proto interface with exactly two methods:

```capnp
interface Add {
  write @0 (a :Float64, b :Float64) -> stream;   # input ports = write params
  done  @1 () -> (out :Float64);                 # output ports = done results
}
```

- `Done` returns the result **and resets** the server. Ordinary primitives keep no state across evaluations. Retention is explicit composition with a `store.*` node.
- A primitive knows nothing about the graph: no downstreams, node IDs, sinks, or `any` payloads. Routing belongs to the compiled `Program`.
- Constructor is `NewX()`, server type is `XServer`, and errors go through `errnie.Error(errnie.Err(category, "pkg: msg", err))`.
- Schema field names **are** the Flume port names. Do not add alias layers.
- Resource owners such as sockets and listeners may live across evaluations. They must not stand in for algebraic state.
- `nomagique/README.md` still describes the old `types.Value` closure model. It is stale; follow `ARCHITECTURE.md`.

### Adding or changing a node

1. Write or edit the `.capnp` file in its package (`$Go.package` / `$Go.import` annotations; `using Go = import "/go.capnp"`).
2. Regenerate the Go bindings with `capnp compile -I $(go list -m -f '{{.Dir}}' capnproto.org/go/capnp/v3)/std -ogo`. **Compile every schema in the package in one invocation.** The generator emits one registration function per package, so separate runs create duplicates.
3. Implement `XServer` with `Write` and `Done`, plus `NewX`. Add a GoConvey test that drives it through `X_ServerToClient` and checks the reset between evaluations. `nomagique/arithmetic/add_test.go` is the reference.
4. Run `make catalog`. It regenerates `nomagique/compiler/factories_gen.go` and `frontend/src/components/flume/flume-config.generated.ts`. Do not hand-edit either file.

## Current focus: training in the graph model

The goal is to restore the precursor training system entirely as composed graphs (manifests of generic nodes), not as Go application code. `TRAINING.md` is the behavioural contract, and its "Where this stands" table is the status of record. `docs/TRAINING-REPAIR-DESIGN.md` is the proposed maths for grading, pair evidence, remapping and region tokens. One point is still pending review: a lexicographic versus an additive remapper objective. Do not settle it in code. The label question is settled: decisions are graded by executable PnL (see below).

The manifests are wired as follows:

| Manifest | Role | Wired into `training.json`? |
| --- | --- | --- |
| `capture.json` | WebSocket → `store.Capture` → Iceberg `raw_frames_v3`. Its own process, no model evaluation | separate program |
| `archive_scan.json`, `archive_records.json` | `IcebergScan` (via the catalog HTTP client) → Arrow → records | yes |
| `training.json` | root: `store.Tape` (order and dedupe per session) → `temporal.Mine` (excursions → `excursion_fragments_v1`) → `store.Grid` + all signal sub-graphs → replay → grade | root |
| `training_replay.json` → `training_fragment.json` | record cursor over `store.Sequence`. Selects the applicable A/B/C fragments per record and keeps causal observations apart from future labels | yes |
| `training_grade.json` | A/B/C fragment selection (WAIT/ENTER/EXIT labels per cursor) | yes |
| `paper_exchange.json` (wired as `paper_exchange` in `training.json`) | **the grade**, composed of nodes: `paper.Book` (L3 replay, SDK book + checksum) and `paper.Sweep` (walk levels), `arithmetic.Decimal*` for money, account state (cash, holding, resting order, pair record per symbol) in one `store.Radix` written back as feedback. Orders rest until the next L3 frame; 20% of cash per entry; round-trip PnL. Takes one ordered `event` stream (observations and decisions); fragment ENTER-at-B/EXIT-at-C decide until the trie exists; round trips → `paper_round_trips_v1` | yes |
| `capture.json` + `live_level3.json` | capture program: `live_spot` (public) and `live_level3` (token per connection, one subscription per Kraken heartbeat via `store.Queue`) into one gathered `store.Capture` session → `raw_frames_v3` | separate program |
| `training_pair.json` → `training_pair_step.json` | pair-evidence stage of the remapper (sign and magnitude sufficient stats in `store.Radix`) | **not yet** |
| `training_reinforce.json` | grade → `cognition.Attractor` + `statistic.Tally` into the trie | **not yet** |

Grading is **executable PnL**, not label accuracy. The balance is training state: losses compound, and a wallet below the venue minimum is a consequence, never skipped or resized. The warehouse is the SeaweedFS REST catalog `symmtables` (`http://iceberg.seaweed.home.arpa`, anonymous S3 at `http://s3.seaweed.home.arpa`), declared in each table's `catalog`/`properties`; `tables` imports `iceberg-go/io/gocloud` for `s3://`. Throughput is the current limit for training on real L3 archives (see TRAINING.md).

Live Kraken facts (measured 2026-09-23): Level 3 is on `wss://ws-l3.kraken.com/v2` with a token from the `L3_API_KEY`/`L3_API_SECRET` env credentials; snapshot subscriptions burst to 40 and then refill (one per 2 s never rejected), so admission must be paced and rejections requeued. Public `AssetPairs` no longer publishes fees; the real taker fee (0.80% below 2,500 USD 30-day volume) comes from private `TradeVolume`, keyed by legacy pair names (`XXBTZUSD`). `paper_exchange.json` currently holds the measured 0.80% as a visible `store.Constant`; feeding it from `TradeVolume` needs the legacy→v2 join (via `altname` in `AssetPairs` v0 and v1), not built yet. Nonces for these keys are nanosecond-scale.

Graph idioms that matter here: every wired input is required EXCEPT gathered (list) ports, so a node whose inputs all gather runs every pass with whatever arrived — that is how independent events join (such nodes must report an idle union branch, never an empty value). Retained state is written back from descendants as deferred feedback, committed after the evaluation. Branches are `data.Filter` stages whose complement is a `controlflow.Select` on `passed`; alternatives merge through a `Select` with static `test` true. Money travels as JSON-number text and is computed by `arithmetic.Decimal*` on exact rationals (`core.ReadDecimal`/`WriteDecimal`); never the Kraken SDK's `decimal.Mul`/`Div`, whose `BankersRound` turns 99×1 at scale 0 into 100. Nodes embed `*runtime.System`, construct with `NewX(ctx)` and report through `server.Error(errnie.Err(...))`.

The following are not built yet: metric 2D coordinates, the remapper settling gate, region tokens, the trie's sensory keys, the connection from token to reinforce, emitting flat or non-event fragments, and the live paper process.

Invariants that are easy to break here:
- Future B/C/D values reach the grader inputs only, never the causal token path.
- The truth class is recorded for every resolved example, including ones the model got wrong. That is the bootstrap for an empty trie. A re-recorded example must not inflate support.
- A missing metric is unknown, never zero or last-value. Never widen class boundaries to fill an empty class.
- New behaviour is a node (a generic primitive) plus wiring in a manifest. Keep domain vocabulary out of `nomagique/` outside `financial/`.

## Tests

- GoConvey BDD nesting. `<file>.go` pairs with `<file>_test.go`, and test names mirror the production function.
- `nomagique/compiler` has manifest tests (`manifest_test.go`, `manifests_agree_test.go`, `system_test.go`) that compile the real embedded graphs. Run them after touching any manifest JSON or a schema that a manifest wires.
- Always pass `-p 1` for packages that touch Metal or physics.

## Related docs

- `ARCHITECTURE.md`: runtime and compiler design (the compile phases, the evaluation frame, fan-in/out, hot recompile).
- `TRAINING.md`: the precursor learning pipeline and what is still unimplemented.
- `UI.md`: how the React library is reflected into UI nodes and routes.
- `docs/TRAINING-REPAIR-DESIGN.md`: in-progress training repair design.
- `LEGACY.txt`: a dump of the removed pre-graph codebase (84k lines). Historical only.
