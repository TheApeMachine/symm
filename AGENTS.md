# AGENTS.md

## Objective

**Maximize the wallet. Minimize the time to do so.**

Build a principled market system from honest market data. Statistical thresholds, horizons, baselines, confidence, retention, and regime decisions must come from the data and its measured uncertainty—not arbitrary constants.

## Scope & Completion

- Touch only files required by the task.
- If required work exposes obsolete code, fields, methods, files, schemas, or compatibility paths in that ownership chain, **delete them**.
- Do not preserve old code “just in case.” Git is the history.
- Do not add a second implementation beside the old one. Replace the old path completely.
- Do not roam into unrelated cleanup. Mention unrelated throughput hazards rather than expanding scope.
- Once the requested work is correct and tests pass, **stop**.

“Pre-existing” does not excuse a violation encountered inside the code you must change.

---

## Ownership & Composition

### Refactors must be subtractive

A refactor succeeds only when the system has fewer ownership paths and less
caller knowledge. Creating more files, types, constructors, wrappers, or fields
is not composition by itself.

Before implementation, inspect the ownership graph and make an internal deletion
plan. For every proposed owner, identify:

1. The state it will exclusively own.
2. The exact fields, methods, types, constructors, and files that disappear.
3. The callers that become shorter.
4. The dependencies those callers no longer need to know.

If nothing disappears or no caller becomes shorter, do not add the abstraction.
Extraction without deletion is prohibited. A new owner must immediately replace
the old path, including its state, behavior, wrappers, tests, and comments.

Do not create package-name containers such as `broker.Broker`,
`strategy.Strategy`, `runtime.Runtime`, or `store.Store`. Application composition
roots belong at the application wiring boundary (`cmd`, `system`, etc.), not
inside the domain package they assemble. Do not collect every dependency into a
new bag and pass that bag around.

There is no arbitrary target number of files or types. Public API surface must
not grow unless unavoidable; constructor fan-out must shrink; callers must become
shorter; duplicate ownership paths and obsolete files must disappear; total
implementation ceremony must decrease. If a diff mostly adds owners while
retaining the old behavior, stop and redesign before continuing.

Increasing, reducing, and exiting are operations of one position regulator, not
separate objects. One pending order and one cumulative reconciliation path own
their common state. The agent owns its positions directly; do not recreate Desk
as a Collection, Broker, or another forwarding facade.

A good abstraction hides a decision. A bad abstraction merely moves code.

### One concept, one owner

A type owns one cohesive concept: its state, lifecycle, invariants, and operations.

If behavior can be named independently and can change independently, give it a real owner and compose it.

**Bad**
```go
type Position struct {
    increaseOrder *Order
    reduceOrder   *Order
    exitOrder     *Order
    guardianRing  Ring
    guardianSlot  []Event
    // dozens of unrelated responsibilities...
}
```

Use `broker/position/regulator.go`, `guardian.go`, `store.go`, and `recovery.go`
for actual package boundaries. Underscores do not create namespaces. The type
is `position.Regulator`, not `position.Position`.

State moves with the behavior that owns it. Do not leave fields on a parent while moving only methods elsewhere.

### Use existing dependency semantics

Before implementing normalization, alias resolution, precision handling,
formatting, parsing, retries, or protocol translation, inspect the dependency
that owns the external protocol. Call its existing operation directly.

Use Kraken's `Normalizer.Name`, `FormatPrice`, and `FormatSize`. Do not recreate
them with alias lists, `QtyPrecision`, manual scales, or another Precision type.
Use SDK Decimal operations and assign existing decimals directly. Do not add raw
`big.Rat` pricing, zero-plus-value assignments, wire snapshots, or intermediate
copies of state that already has an owner.

Instrument owns venue facts without depending on Price. Price owns fees and
economic calculations. Its callers must not configure another Pricing object,
pass its own fees back to it, or fetch pair metadata just to supply a symbol.

### Root objects wire; owners work

`Agent`, `Solver`, `Desk`, `Broker`, `Execution`, `Workspace`, etc. are composition roots/coordinators. They must not become dumping grounds.

A composition root at the application wiring boundary may collapse repeated construction:

```go
type application struct {
    Price      *Price
    Instrument *Instrument
    Positions  *Positions
}
```

Expose composed owners directly. Do not recreate them through forwarding methods.

### Zero proxy methods

**Bad**
```go
func (desk *Desk) Price() *Price {
    return desk.price
}

func (desk *Desk) Cash() *decimal.Decimal {
    return desk.balance.Cash()
}
```

Use the real owner directly.

### Encapsulation must reduce code

Encapsulation exists to remove knowledge from callers.

If using an abstraction requires callers to fetch its dependencies, validate them, construct another helper, configure it, and then call the operation, ownership is incomplete.

**Bad**
```go
fee := price.Fee(symbol)

var pricing Pricing

err := pricing.SetFee(fee.Fee)

total := pricing.Total(new(big.Rat), amount.Rat(), true)
```

**Good**
```go
total, err := price.Total(symbol, amount, BUY)
```

A cohesive operation should normally be consumable in one call.

Do not replace five lines of business logic with ten lines of abstraction ceremony.

### Money is Decimal

Money, price, quantity, fee, cost, basis, and PnL use SDK Decimal. Keep the
venue's authoritative cumulative cost, quantity, and fee directly. Use its
AvgPrice when supplied. Never recover authoritative cost by multiplying a
rounded VWAP back into quantity.

Allocate finite basis and fees on partial sales, then retain the remainder by
subtraction: allocated + remaining must equal original exactly. Raw big.Rat is
reserved for non-monetary mathematics whose intended domain is rational.

### One canonical owner for shared invariants

Price, fee, sizing, normalization, construction, protocol setup, persistence rules, and other shared invariants get one canonical owner.

Consumers must not reproduce or partially reproduce them.

An internal helper is fine. A second public owner is not.

### Construction is ownership

Do not propagate the same constructor dependencies through dozens of callers.

If every caller needs `api`, `price`, `instrument`, `store`, `balance`, etc., create the concrete owner/factory that already knows them.

### Interfaces require a real boundary

No interface for architecture theatre. Use an interface only for genuine substitution, multiple implementations, or an existing protocol.

Prefer concrete composition.

### File splitting is not composition

A receiver may span files when it is still one cohesive concept. Splitting unrelated method families across `foo.go`, `foo_x.go`, `foo_y.go` does not create ownership.

Around 200 implementation LOC should trigger an ownership review, not a mechanical file split.

---

## Market Math: No Magic

### No arbitrary statistical constants

Do not hardcode:

- market time horizons;
- observation/sample windows;
- baseline spans;
- retention lengths;
- sigma/confidence multipliers;
- regime thresholds;
- probability cutoffs;
- unexplained ratios;
- bare multipliers such as `*2`;
- arbitrary denominators;
- fixed polling intervals masquerading as market time.

Putting a value in config or giving it a descriptive constant name does **not** make it derived.

“Declared operating choice” is not an exemption.

**Bad**
```go
const horizon = 32
const sigma = 2.0
const memory = 512.0
const delay = 30 * time.Second
```

**Good**
```go
reading := baseline.Step(value)
threshold := reading.Bound
horizon := reading.StableSpan
```

Use the statistics already measured by the system: support, variance, SNR, event rate, stability, maturity, information content, or another mathematically justified observable.

Structural constants are allowed only when imposed by an external protocol, representation, physical/resource boundary, or exact mathematical identity. They must not determine market belief.

### No fallback fakery

Missing, invalid, immature, or undefined evidence stays missing, invalid, immature, or undefined.

**Bad**
```go
if threshold <= 0 {
    threshold = 0.02
}
```

**Good**
```go
if threshold <= 0 {
    return errnie.Error(errnie.Err(
        errnie.Validation,
        "threshold is not defined",
        nil,
    ))
}
```

Never silently substitute a convenient default.

### Never check NaN or Inf

Do not use `math.IsNaN`, `math.IsInf`, “finite” gates, clamping, zero substitution, or skipping to keep invalid numerical state alive.

Let invalid mathematics surface so its cause gets fixed.

Do not solve the symptom.

---

## Streaming & Hot Paths

Market processing is streaming.

- A `Step` gets one observation and one opportunity to process it.
- Do not accumulate/snapshot/copy history when sufficient statistics can be updated.
- Use adaptive `nomagique` state where appropriate.
- `Step` must continue to produce its output contract from the first observation; maturity/support expresses uncertainty.
- Protect throughput.

Market/guardian execution paths must not wait on SQLite, disk, telemetry, snapshots, or a blocking persistence queue.

Durable execution facts may be processed asynchronously, but:

- they must never be silently dropped;
- persistence failure must be explicit;
- new-risk admission must stop when durability cannot keep up;
- existing position protection must remain operational;
- causal ordering must be preserved;
- persistence consumers operate on immutable transition facts, not racing mutable state.

A slow persistence consumer must not become a hidden gating consumer for the execution fast path.

---

## Control Flow

- Guard clauses and early returns.
- **No `else` blocks.**
- Maximum two nested `if` levels.
- Do not hide deep branching in meaningless helper functions.
- Every non-leading `if` has an empty line above it.
- No single-character variable names except `t *testing.T` and `b *testing.B`.

---

## Errors

Errors are part of the system state.

- Error variables are named `err`.
- Never ignore an error.
- Never discard an error with `_ =`.
- Never `continue` after a failed scan/decode/persistence operation.
- Do not turn unexpected failure into `nil`, zero, empty state, or a fallback.
- Returned errors pass through `errnie.Error`.
- Construct categorized errors with `errnie.Err`.
- Log through `errnie`, not `log.Printf`, `log.Fatal`, ad-hoc printing, etc.
- Cleanup errors (`Close`, `Rollback`, `Flush`, `Detach`, etc.) must be handled.

```go
if err != nil {
    return errnie.Error(errnie.Err(
        errnie.IO,
        "component: descriptive failure",
        err,
    ))
}
```

---

## Tests

Tests are production architecture too.

- `<file>.go` → `<file>_test.go`.
- Test functions mirror the production method/function being tested.
- Use GoConvey with BDD nesting.
- Shared fixtures/builders/mocks have one test owner.
- Constructor changes should normally change one shared fixture, not dozens of tests.
- Market behavior uses multi-leg replay through `tests/market`; never single-spike fixtures.
- Test real ownership boundaries and failure behavior, not implementation trivia.

---

## Replacement Rule

When ownership moves:

1. move the state;
2. move the behavior;
3. update callers to use the new owner directly;
4. delete old fields;
5. delete old methods;
6. delete forwarding wrappers;
7. delete obsolete helpers/interfaces/files;
8. delete stale tests/comments/schema fields;
9. verify there is one path left.

A refactor that leaves both old and new paths is incomplete.

The expected result of good composition is usually **less caller code, fewer dependencies, fewer methods, and fewer ways to do the same thing**.
