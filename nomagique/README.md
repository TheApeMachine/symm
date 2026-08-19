# nomagique

`nomagique` is a universal numeric reducer engine. Everything it computes is a
plain `float64` living in an interned slot of a value-type `Frame`; every
computation is a pure `Primitive` transition from one state `Frame` to the next;
and every signal is just a composed numeric unit.

The name is the thesis: **no magic numbers**. A window size, a baseline
half-life, an adaptation rate — none of these are constants a caller hardcodes.
They are derived from the data itself (its event-time spacing, its dispersion,
its stability), so a pipeline grows, slides, and shrinks itself without human
intervention.

## The three contracts

### 1. `Frame` — the universal numeric payload

A `Frame` is the only data type that flows between primitives. It is a fixed-size,
value-type bag of `float64` slots addressed by interned `Symbol`s. Every value is
just a number, so a "quantity", a "price", a "z-score", or a "stability" are all
the same thing: a slot and a value.

### 2. `Primitive` — the universal reducer contract

```go
type Primitive func(
    state Frame,
    input Frame,
) (nextState Frame, output Frame, err error)
```

A primitive reads slots out of `input`, reads and updates `state`, and writes its
result into `output`. It owns no goroutines, no locks, and no magic numbers. All
state lives in the `Frame` passed in; all state that must survive to the next
tick is written back in `nextState`.

### 3. Input contracts — the shared numeric vocabulary

Because every value is a number, primitives compose only when they agree on
which slots they read and write. `nomagique/types` declares those shared slots
(`types.Quantity`, `types.AlphaQuantity`, `types.BetaQuantity`, `types.AlphaPrice`,
`types.BetaPrice`, `types.EventTimeSec`, `types.EventTimeNsec`, `types.Span`), so
the output of one preset plugs directly into the input of another without
signal-specific renaming. A producer that lifts raw market rows into this
vocabulary only puts the right slots; a consumer only `Get`s the same slots.

## The four composition patterns

### Pattern A — straight pipeline (`Pipe`)

Each stage transforms the input for the next stage:

```go
nomagique.Pipe(algo.Ignition, algo.Hawkes)
```

### Pattern B — fan-out (`Pipe` + `Fork`)

A common base (a `Window`, a conditioned series) fans out into several
estimators that each add their own slots to the shared output. `Fork` runs both
reducers against the same input, lets the second observe the state changes made
by the first, and merges both outputs into one `Frame`:

```go
nomagique.Pipe(
    temporal.Window,
    nomagique.Fork(
        statistic.Baseline,
        nomagique.Fork(statistic.Velocity, statistic.ZScore),
    ),
)
```

### Pattern C — closed-loop control (`Configure`)

A control parameter computed by one primitive feeds a consumer on the same
tick, but the consumer still receives the original input as its primary data:

```go
nomagique.Configure(
    statistic.Baseline, // producer: adapts baseline, computes Span
    nmtypes.Span,       // the control channel
    temporal.Window,    // consumer: sizes itself to Span
)
```

`Configure` runs the producer, extracts the named channel, overlays it on the
original input, runs the consumer, and merges the producer's output back in so
no metric (baseline, stability, efficiency) is discarded. Feedback across time
steps is carried naturally in `state`: baseline stability at step t−1 sizes the
window at step t.

### Pattern D — the living `Number`

`Number` is the top-level composer that turns a pipeline of primitives into one
self-adapting numeric unit. It is keyed, so every stream identity (a market
symbol, a peer pair) owns an isolated instance with its own window, baseline,
and event clock:

```go
number := nomagique.NewNumber[string](primitives...)
output := number(symbol, input)
```

`NewSingle` is the unkeyed unit behind each key, and `Pipe` remains the
stateless by-value composition path when callers manage their own `Frame` state.

## The adaptive window and baseline

The adaptive machinery is a genuine closed loop, not a configured horizon:

1. **Bootstrap** — the window starts at one sample ("now") and earns memory by
   doubling only when more samples arrive than the current capacity.
2. **Measure** — the baseline is an event-time EMA whose half-life is the data's
   own inter-arrival span; stability is the ring's relative dispersion
   (`1 − largestResidual / range`), always in `[0, 1]`.
3. **Feed back** — on a stability dip the window doubles to gather more
   evidence; at perfect stability it shrinks toward the samples it actually
   retains; otherwise it slides at its current size. The verdict is emitted as
   `types.Span` and adopted by the window on the next tick.
4. **Score** — `ZScore` and `Deviation` then score the departure against the
   adapted baseline, so downstream logic sees standardised truth, not raw depth.

## Repository layout

### Root files

- `doc.go` – package documentation
- `number.go` – `Number[Key]` (keyed composer), `Single`/`NewSingle`
- `frame.go`, `frame_test.go` – `Frame`, `Get`/`Put`/`MustGet`, merge semantics
- `primitive.go` – `Primitive`, `Step`, `Pipe`, `Fork`, `Configure`, `Identity`
- `stream.go`, `stream_test.go` – single-writer stream execution
- `keyed_stream.go`, `keyed_stream_test.go` – per-key stream isolation
- `symbol.go` – symbol interning (`Intern`/`MustIntern`)
- `named.go`, `named_test.go` – named access helpers
- `samples.go` – generic sample slots (`MaxSamples`)
- `error.go` – error helpers
- `example_test.go` – usage examples

### Sub-packages

- **algo/** – Hawkes, Ignition, Ladder primitives
- **calculus/** – arithmetic, nonlinear, stateful reducers and shared symbols
- **causal/** – causal tables
- **data/** – bounded event-time series
- **learning/** – feature detectors, predictive dynamics, RLS, resonance
- **logic/** – gating primitives
- **mcts/** – Monte Carlo tree search primitives
- **probability/** – geometric mean and calibrators
- **statistic/** – baseline, z-score, deviation, lift, velocity, extremes
- **temporal/** – window, clock, duration, interval
- **transport/** – lock-free queues, map/reduce, ring buffers
- **types/** – `Frame`/`Primitive`/`Symbol` definitions, input contracts,
  `Measurement`/`Metric`/`Descriptor` boundary metadata
- **utils/** – numeric helpers

## The consumer blueprint

Every signal follows exactly the same shape. It is a nomagique pipeline plus
the two boundary adapters that frame it: input conversion and output push.

```go
type Signal struct {
    ctx    context.Context
    cancel context.CancelFunc
    thesis *types.Thesis
    number nomagique.Number[string]
}

func NewSignal(ctx context.Context, thesis *types.Thesis) *Signal {
    ctx, cancel := context.WithCancel(ctx)

    signal := &Signal{
        ctx: ctx, cancel: cancel, thesis: thesis,
        number: nomagique.NewNumber[string](
            nomagique.Pipe(
                statistic.ExtractDepth,
                nomagique.Configure(statistic.Baseline, nmtypes.Span, temporal.Window),
                nomagique.Fork(statistic.ZScore, statistic.Deviation),
            ),
        ),
    }

    signal.run()
    return signal
}

func (signal *Signal) run() {
    for {
        select {
        case <-signal.ctx.Done():
            return
        default:
            signal.thesis.Symbols.Range(func(_ any, value any) bool {
                symbol, _ := value.(*types.Symbol)
                for ticker := range symbol.MarketTickers(types.SourceLiquidity) {
                    // 1. Input conversion: lift a raw exchange tick into the
                    //    generic nomagique vocabulary.
                    input := nomagique.Frame{}
                    input.Put(nmtypes.AlphaPrice, ticker.Bid.Float64())
                    input.Put(nmtypes.BetaPrice, ticker.Ask.Float64())
                    input.Put(nmtypes.AlphaQuantity, ticker.BidQty)
                    input.Put(nmtypes.BetaQuantity, ticker.AskQty)
                    input.Put(nmtypes.EventTimeSec, ...)
                    input.Put(nmtypes.EventTimeNsec, ...)

                    // 2. Step the isolated per-symbol number.
                    output, err := signal.number(symbol.Symbol, input)
                    if err != nil { /* errnie + continue */ }

                    // 3. Output push: project the numeric output into a
                    //    boundary Measurement.
                    symbol.Measurements.Push(nmtypes.NewMeasurement(
                        uuid.NewString(), signal.Name(),
                        ticker.Timestamp.UnixNano(),
                        ticker.Timestamp.UnixNano(),
                    ).AddMetrics(
                        nmtypes.NewMetric("executable_touch_depth",
                            output.MustGet(nmtypes.Quantity), nmtypes.Descriptor{
                                Unit: nmtypes.UnitPrice,
                                Timescale: nmtypes.TimescaleInstantaneous,
                            }),
                        // ...one NewMetric per harvested slot
                    ))
                }
                return true
            })
        }
    }
}
```

## Example

```go
import (
    "github.com/theapemachine/symm/nomagique"
    "github.com/theapemachine/symm/nomagique/calculus"
)

input := nomagique.Frame{}
input.Put(calculus.SymbolLeft, 3)
input.Put(calculus.SymbolRight, 4)
_, output, err := nomagique.Step(calculus.Sum, nomagique.Frame{}, input)
// output.MustGet(calculus.SymbolResult) == 7
```

## License

Part of the `symm` monorepo. See repository root for licensing details.
