You are performing a repository-wide architectural correction of SYMM.

Read the supplied `AGENTS.md` first. It is authoritative.

This is explicitly a replacement/refactoring task, not an additive feature task. You are authorized and expected to delete obsolete files, types, fields, methods, functions, interfaces, schema fields, compatibility code, constructors, wrappers, comments, and tests when their ownership has been replaced.

Git already preserves history. Keeping dead code “for safety” is a failure of this task.

All remediation sections below are mandatory. Their order is not a priority ranking.

# Required outcome

Bring the current repository into compliance with `AGENTS.md`, including both the already identified violations and other instances of the same violation classes that you discover while working.

The finished architecture must have:

- one clear owner for price/fee/execution economics;
- materially smaller and more cohesive `Desk`, `Position`, and persistence ownership;
- no old parallel ownership path left behind;
- no proxy/passthrough APIs added to preserve the old shape;
- materially less constructor/call-site ceremony;
- no blocking SQLite/persistence work on the market/position guardian path;
- no silent persistence loss;
- honest degraded-recovery provenance;
- data-derived statistical horizons/thresholds instead of fixed market constants;
- no NaN/Inf sanitization;
- no `else`;
- no ignored errors;
- repository-wide compliance with the naming/control-flow/error rules in `AGENTS.md`;
- tests covering the new ownership and failure semantics.

Do not “solve” this by adding adapters around the existing architecture.

The objective is fewer concepts at each call site and one path for every invariant.

---

# PRICE / FEE / EXECUTION ECONOMICS: ESTABLISH ONE CONSUMABLE OWNER

The current code already claims that `broker.Price` owns fee tiers, ticker state, precision, and money math, but ownership is incomplete.

Relevant current areas include:

- `broker/price.go`
- `broker/price_fee.go`
- `broker/quote.go`
- `broker/entry_economics.go`
- `broker/desk.go`
- `broker/position.go`
- `broker/lot_increase.go`
- `cmd/learning_desk.go`
- `cmd/learning_admission.go`

Current symptoms include:

- exported/free calculation functions such as `Notional`, `UnitPrice`, `Prorate`, `ReturnFraction`, `OrderQuantity`, etc.;
- callers fetching a fee and then constructing/configuring `Pricing`;
- repeated fee validity checks in callers;
- `Pricing` being independently constructed in multiple operations;
- fee callbacks being passed separately from the price owner;
- callers knowing normalizer/book/fee details that the pricing owner already knows;
- PnL and return requiring several chained price operations;
- `Desk.Execute` revalidating fee details that executable pricing should own;
- `learningDesk` carrying separate book/instrument/fee knowledge instead of consuming the economic owner ergonomically.

Correct this.

Use **one concrete public economic owner**. Prefer evolving the existing `broker.Price` because it already owns the relevant state unless inspection demonstrates a genuinely better single owner.

Do NOT create parallel public concepts such as `Price`, `FeeCalculator`, `ExecutionPricing`, and `Economics` which callers then have to orchestrate.

Internal private helpers are allowed when they simplify implementation, but callers must see one owner.

The desired caller complexity is equivalent to:

```go
entry, err := price.EntryCost(symbol, quantity)
```

```go
quantity, err := price.Quantity(symbol, cash)
```

```go
surface, err := price.Surface(symbol, sellable, at)
```

```go
valuation, err := price.Value(pair, holding)
```

```go
notional := price.Notional(reference, quantity)
```

Names may differ if the resulting API is clearer. The point is ownership and ergonomics.

A caller must NOT need to do this:

```go
fee := price.Fee(symbol)

var pricing Pricing

err := pricing.SetFee(fee.Fee)

value := pricing.Total(...)
```

A caller must NOT fetch a fee merely to pass that fee back into another price method.

A caller must NOT reproduce the price owner's validation rules.

A caller must NOT carry a `func(string) *TradeVolumeFee` callback when it can consume the concrete price owner.

Consolidate:

- fee lookup/cache;
- fee validation;
- normalization needed for pricing;
- exact fee application;
- exact notional arithmetic;
- order quantity/lot rounding;
- entry cost;
- depth-aware executable entry;
- liquidation surface;
- exit value;
- friction/slippage calculation;
- PnL;
- return;
- VWAP/unit-price calculations;
- fee-inclusive break-even calculations.

Keep exact rational arithmetic internally where appropriate.

Do not round earlier merely to make the API simpler.

Remove obsolete exported helpers once callers use the canonical owner. Do not leave compatibility functions forwarding to the new methods.

The architecture is successful only if consumers become shorter.

---

# CONSTRUCTION: STOP PROPAGATING THE WHOLE BROKER GRAPH

The same dependencies currently appear repeatedly in constructors such as `NewDesk`, `NewRecovery`, `NewPosition`, and `newLearningDesk`.

Repair the construction boundary.

A concrete broker composition root is acceptable and likely desirable if it materially removes constructor fan-out, for example:

```go
type Broker struct {
    Price      *Price
    Instrument *Instrument
    Balance    *Balance
    Positions  *Positions
    Desk       *Desk
}
```

This is illustrative, not a requirement on names.

Important:

- the composition root wires;
- composed owners do the work;
- expose owners directly;
- do not add `Broker.Price()`, `Broker.Balance()`, etc. forwarding methods;
- do not turn the root into a service locator containing unrelated behavior;
- do not introduce interfaces merely to reduce constructor arguments.

Create a real position collection/factory owner if it can own both the position map and the shared dependencies required to construct positions.

`NewPosition` should not require every caller to repeatedly provide the same API/instrument/price/balance/store graph.

The owner constructing positions should already know the incidental wiring.

---

# DESK: REDUCE IT TO COORDINATION

`broker.Desk` currently owns or coordinates too many unrelated concepts:

- position collection;
- embedded persistence;
- API;
- instrument registry;
- price;
- balance;
- equity state/versioning;
- account locking;
- recovery;
- lifecycle recorders;
- periodic balance/equity refresh;
- market routing;
- order execution;
- instrumentation.

Review each field and method against the ownership test in `AGENTS.md`.

Extract only real owners—types that remove multiple fields/methods and own a coherent lifecycle.

Do not create one-field wrappers.

Likely ownership candidates include account/equity publication and position collection/construction, but choose based on actual state and invariants.

Delete proxy methods such as the current `Price()`, `Balance()`, `Instrument()`, `Recovery()`, and similar passthroughs when callers can depend on the real owner.

Review `Cash()` under the same rule.

Do not replace these proxies with differently named proxies.

Remove unused state such as `balanceRefresh` if inspection confirms it has no owner/use.

Investigate `StepLevel3Epoch`/`stepLevel3`: the epoch argument is currently passed through but does not appear to participate in the shown reducer call. Either make epoch ownership real at the component that needs it or remove the false API/claim. Do not retain a parameter/comment that implies semantics the implementation does not provide.

`Desk` should finish as a coordinator, not the place every broker concern is convenient to reach.

---

# POSITION: COMPOSE REAL SUB-OWNERS

`broker.Position` currently combines:

- lot state;
- entry order state;
- increase order state;
- reduction order state;
- exit order state;
- execution deduplication;
- API/instrument/price/balance/store dependencies;
- persistence callbacks;
- guardian transport;
- guardian slots;
- guardian watermark;
- exit synchronization/claiming;
- recovery metadata;
- telemetry projection;
- close lifecycle.

Some composition already exists through `LotIncrease`. Finish the job.

At minimum, evaluate and implement real owners for:

- guardian transport/lifecycle;
- lot increase;
- lot reduction;
- lot exit.

For example, a `PositionGuardian` should own its disruptor, slots, handler, publication state, watermark, startup, and close lifecycle.

If `LotReduction` exists, it owns its order, sequence, fill reconciliation, cancellation/state.

If `LotExit` exists, it owns its order/result, claim/synchronization, execution, terminal reconciliation.

Do not leave `ReduceOrder`, `ExitOrder`, sequences, mutexes, and claims on `Position` after moving their behavior.

Do not add `Position.PublishGuardian()` as a wrapper that merely calls `position.Guardian.Publish()`.

Call the owner directly.

`Position` should represent the lot and coordinate the sub-owners that genuinely operate on that lot.

Delete stale stop-loss/protection comments that no longer describe the implementation. In particular, inspect comments claiming `onLevel3` feeds a stop-loss or protection geometry when the current code no longer does so.

---

# POSITION PERSISTENCE: REMOVE THE GOD RECEIVER

The current `PositionStore` receiver spans at least:

- `broker/position_store.go`
- `broker/position_store_writer.go`
- `broker/trade_store.go`

It owns several distinct concerns:

- open-position persistence;
- asynchronous writer queue/fences;
- SQLite transaction batching;
- writer failure state;
- trade journal;
- schema management;
- legacy migration.

This is exactly the “receiver split across files is not composition” problem.

Refactor it into concrete owners with real state/lifecycle boundaries.

Do not mechanically rename pieces while keeping one shared god object.

A valid result might contain concepts equivalent to:

- an ordered persistence writer;
- an open-position repository/read model;
- a trade journal;
- a small composition owner wiring them to the same database.

Names are not prescribed.

Consumers should still perform one semantic operation per transition. They should not orchestrate writer/repository/transaction details.

---

# PERSISTENCE MUST NOT BLOCK THE GUARDIAN

The current position persistence queue can block when full.

Calls to persistence also occur in execution transition paths such as:

- `LotIncrease.Apply`;
- `Position.onExecution`;
- `Position.applyReduceFill`;
- `Position.closeFill`;
- position save/trade-journal paths.

Remove storage latency from the guardian/execution state-transition path.

Hard requirements:

1. The guardian must never wait for SQLite, a transaction, a disk flush, or a full persistence channel.

2. Do not solve this by dropping authoritative execution facts.

3. Do not make a slower persistence consumer a gating consumer of the same Disruptor if that can eventually prevent publication/reservation on the fast path.

4. Do not read mutable `Position`/`Holding` state asynchronously and hope it still represents the transition being persisted.

5. Emit an immutable persistence fact representing the transition that just occurred.

6. Preserve causal ordering.

7. Persistence failure must become explicit system health.

8. When persistence cannot safely keep up, stop admission of **new risk** before placing more orders.

9. Existing open-position protection/execution reconciliation must continue to run.

10. Recovery must be able to identify when authoritative local history was not durable and must not pretend otherwise.

The external review suggested a secondary Disruptor consumer. Do not copy that prescription blindly. First understand the exact gating/overwrite semantics of the Disruptor implementation. Use it only if it satisfies all invariants above.

Similarly, do not create a bounded queue with a `default:` branch that silently sheds position facts.

There is an unavoidable durability/latency tradeoff. Represent it honestly:

- no blocking database I/O on the guardian;
- no silent loss;
- persistence degradation is observable;
- new risk shuts down;
- exchange-authoritative recovery remains available and explicitly degraded.

Add tests proving these semantics.

---

# REMOVE LEGACY/ORPHANED POSITION PROTECTION PATHS

The current persistence code still contains obvious old ownership vocabulary and fields.

Inspect and remove obsolete remnants including, where no longer used by the current model:

- `"delete stoploss for ..."` descriptions;
- `trigger_reason`;
- `trigger_mark`;
- `floor`;
- `peak`;
- `profit_line`;
- `locked`;
- comments referring to old stop-loss/protection ownership;
- legacy migration code whose only purpose is preserving an abandoned architecture;
- compatibility shims around old position databases.

`AdoptLegacyStore` deserves explicit review against the new AGENTS replacement rule.

Do not preserve obsolete schema/API merely because old code once wrote it.

If persistent current data genuinely requires a one-way schema migration, give that migration one explicit owner and migrate to the latest schema. Do not keep the old runtime ownership model alive.

After the migration boundary is satisfied, the live system should operate only on the current schema/path.

---

# DEGRADED RECOVERY MUST HAVE HONEST PROVENANCE

Recovery can reconstruct position basis from exchange trade history and sets `DegradedRecovery`.

That is useful, but a boolean stranded on `Position` is not enough if downstream policy treats reconstructed state identically to a fully witnessed current position.

Correct the ownership/provenance model.

Requirements:

- recovered basis remains mathematically accurate to the exchange evidence available;
- reconstructed state must be explicitly distinguishable downstream;
- do not invent missing historical intent, causal evidence, confidence, horizon, or decision provenance;
- do not fabricate a substitute decision to make the data shape convenient;
- policy/learning must not treat reconstructed provenance as equivalent to locally witnessed provenance.

Do NOT implement the external review's suggested arbitrary fixed “variance multiplier.” That would create another magic number.

Prefer existing concepts such as measured support, maturity, authority, provenance, or explicit availability.

If an adopted position lacks the original decision/thesis, represent that fact directly.

A valid design could make the position an explicitly adopted/recovered position whose basis is authoritative but whose original policy provenance is unavailable. The strategy may then manage it only through behavior justified by current evidence rather than pretending the original thesis survived the crash.

If current live evidence can establish a new thesis/ownership, that transition must be explicit and data-derived.

Add replay/recovery tests for both fully persisted and reconstructed cases.

---

# HINDSIGHT: DELETE FIXED MARKET WINDOWS AND FALLBACK POLICY

`hindsight/episode.go` currently contains a `DefaultDiscoveryPolicy` with fixed values such as:

- floor excursion;
- sigma count;
- excursion horizon;
- retrace fraction;
- regime window;
- regime baseline;
- volatility ratio;
- spread ratio;
- depth ratio;
- arrival ratio;
- minimum regime span;
- minimum observations;
- maximum episodes.

`normalised()` then silently substitutes those defaults when supplied policy values are invalid.

This violates the new AGENTS rules.

Replace this model.

Requirements:

- statistical horizons derive from the observed symbol/process;
- thresholds derive from measured distribution/noise/support;
- baseline spans adapt to measured stability;
- insufficient evidence remains insufficient;
- invalid input returns an error/undefined result rather than receiving a convenient default;
- a config value is not accepted merely because it moved out of source code;
- a comment saying “declared operating choice” is not statistical justification.

The discovery result should still report the actual derived threshold/horizon/support used so Hindsight remains inspectable.

Do not hide adaptive choices.

Use existing adaptive/statistical machinery where it is appropriate instead of reimplementing rolling window logic manually.

---

# FORWARD REVIEW: REMOVE THE FIXED POLLING MODEL

`cmd/hindsight_forward.go` declares:

```go
const forwardDelay = 30 * time.Second
```

and drives `reviewer.pass` using `time.NewTicker`.

Note that the current implementation shown does not actually impose a 30-second cutoff on the tape; it polls the current store every 30 seconds and then selects confirmed episodes.

Therefore fix both the fixed cadence and the misleading abstraction.

Prefer an event/data-driven review boundary:

- review when capture progress/new durable observations make new confirmed episodes possible;
- process incrementally from the last reviewed capture coordinate;
- do not repeatedly reread and rediscover the whole captured tape;
- preserve causal identity/order;
- confirmation should come from episode evidence, not elapsed wall-clock delay.

If some scheduler is structurally necessary, it must not masquerade as market time and must not influence episode qualification.

Delete `forwardDelay` and stale comments describing behavior that does not exist.

---

# AUDIT ALL OTHER STATISTICAL MAGIC

Do not stop after Hindsight.

Repository-wide, inspect statistical/model/policy constants and config defaults.

Known examples requiring classification/remediation include:

- `strategy.skillSigma`;
- `strategy.skillMemory`;
- `system.UninformativeDirectionConfidence` if used as a behavioral threshold rather than mathematical indifference;
- `cognition.minimum_switch_confidence`;
- configured baseline halflives;
- signal sample sizes/capacities;
- fixed model retention/window sizes;
- hardcoded sigma multiples;
- hardcoded market-time durations;
- fixed ratios used to decide regimes;
- “default” statistical values supplied by Viper;
- fixed thresholds hidden in constructors/config factories.

Do not mechanically delete every numeric constant.

Classify it.

A constant may remain if it is an exact mathematical identity, protocol requirement, serialization constraint, hardware/resource constraint, or explicit non-statistical operational bound.

It may not remain merely because it is configurable or named.

Statistical belief must be derived from data.

---

# REMOVE NaN / Inf HANDLING REPOSITORY-WIDE

The repository currently contains many explicit finite-value gates, including `math.IsNaN`, `math.IsInf`, `finite`, `validPositive`, `finiteNonNegative`, and similar helpers.

Audit all Go code.

Remove numerical sanitization that:

- rejects NaN/Inf and continues;
- converts it to an unavailable measurement;
- clamps it;
- substitutes zero;
- skips the sample;
- returns a benign false;
- otherwise allows the system to continue after invalid mathematics.

This includes signal, strategy, Hindsight, learning, manifold, Hawkes, and other numerical code.

Do not merely rename the finite helper.

Do not move the same check deeper into `nomagique`.

Let the invalid state surface according to AGENTS.md so the producer defect becomes visible.

Diagnostic formatting should not require special NaN/Inf branching; normal formatting can report the value.

---

# CONTROL FLOW / STYLE: REPOSITORY-WIDE CLEANUP

Apply the AGENTS rules to all Go files touched by this remediation and all additional violations of the same classes you encounter.

In particular:

- remove every `else`/`else if`;
- use guard clauses;
- max two nested `if` levels;
- ensure an empty line before every non-leading `if`;
- remove single-character variable names except allowed test `t`/`b`;
- rename loop variables such as `i`, `r`, etc. descriptively;
- wrap long argument lists cleanly.

Do not create tiny helpers solely to conceal nested branching. A helper must own or express a meaningful operation.

---

# ERRORS: NO MORE SILENT FAILURE

Perform a repository-wide error audit.

Known examples include:

- `_ = desk.PublishEquity()`;
- ignored `disruptor.Close()`;
- ignored transaction rollback/detach/close results;
- ignored `MarkGapped`;
- `RecentTrades` continuing after scan/unmarshal failure;
- `AdoptLegacyStore` treating every `os.Stat` error as “missing”;
- ignored `RowsAffected` errors;
- `log.Fatalf` in `cmd/root.go`;
- error variables named `schemaErr`, `scanErr`, `unmarshalErr`, etc.;
- bare returned errors that bypass the prescribed `errnie` boundary;
- `fmt.Errorf` error construction where `errnie.Err` is the project error owner.

Correct all instances in scope and matching violations discovered repository-wide.

Rules:

- error variable name is `err`;
- no error result discarded into `_`;
- no failed decode/scan/write may silently `continue`;
- use `errnie.Err` for categorized project errors;
- use `errnie.Error` when surfacing/returning errors;
- cleanup errors are handled;
- unexpected failures do not become zero/nil defaults.

Where a callback cannot return an error, give the owning component explicit failure state/cancellation rather than logging and pretending success.

---

# REMOVE PASS-THROUGH / FALSE ABSTRACTIONS

Search for methods whose body is effectively:

```go
return owner.Method(...)
```

or:

```go
return struct.field
```

Delete them unless the receiver itself owns a genuine invariant enforced by that method.

Update callers to use the composed owner directly.

Also search for:

- namespace-only types;
- one-method wrappers;
- interfaces with one concrete implementation and no real boundary;
- helper objects created only to rename another object;
- compatibility wrappers left after a move.

Do not preserve these for API convenience inside this repository.

---

# TEST ARCHITECTURE

Add/update tests according to AGENTS.md.

Use GoConvey.

Tests must mirror implementation files/methods.

Use shared fixtures/builders for repeated broker graphs.

Do not create dozens of hand-built `API + Instrument + Price + Balance + Store + Map` fixture graphs. That would reproduce the construction failure in tests.

Market behavior uses multi-leg replay through the existing `tests/market` infrastructure.

Required behavioral coverage includes:

- canonical price/fee owner produces correct exact entry economics;
- fee lookup/validation is not duplicated by consumers;
- sizing respects venue lot constraints;
- exit/PnL/return use the same canonical economics;
- reconstructed position basis remains correct;
- degraded recovery provenance survives into the policy-facing state;
- persistence transition ordering;
- persistence failure disables new-risk admission;
- a blocked/slow SQLite writer does not block guardian state processing;
- authoritative persistence facts are not silently dropped;
- position increase/reduce/exit owners reconcile cumulative execution correctly;
- guardian close/start lifecycle is owned by the guardian;
- Hindsight discovery derives its selector from the observed data;
- insufficient Hindsight evidence remains insufficient rather than receiving defaults;
- forward review processes newly confirmed episodes without a fixed wall-clock market horizon.

Do not use single-spike market fixtures.

---

# DELETE OLD CODE AS YOU GO

This requirement is important.

Whenever you replace ownership, immediately remove the former path.

Examples:

If price economics moves behind `Price`:
- delete exported old pricing helpers no longer needed;
- delete duplicate validation;
- delete fee callback plumbing;
- delete compatibility forwarding methods.

If guardian transport moves to `PositionGuardian`:
- delete ring fields from `Position`;
- delete `Position.publishGuardian`;
- delete duplicate close/start logic.

If reduction moves to `LotReduction`:
- delete `ReduceOrder` and `reduceSequence` from `Position`;
- delete old reduction methods after callers use the owner.

If persistence splits into real owners:
- delete the old `PositionStore` method families;
- delete obsolete files if they no longer represent an owner.

If Hindsight policy becomes adaptive:
- delete `DefaultDiscoveryPolicy`;
- delete fallback normalization;
- delete obsolete config values.

Do not leave TODOs saying the old path can be removed later.

Do not add `Deprecated:` wrappers.

Do not create aliases to keep the old type names alive unless an actual external public API contract requires them and that contract is demonstrated in the repository.

---

# PERFORMANCE REVIEW DURING THE REFACTOR

This system processes hundreds of symbols and full Level3 streams.

While changing ownership, inspect allocations and blocking behavior.

Particularly verify:

- no per-L3 full-book copies/snapshots;
- no persistence operations on each market mutation;
- no new reflection on hot paths unless already amortized/cached;
- no repeated fee/normalization lookup that can be owned/cache-resolved once;
- no new goroutine per market event;
- no unbounded channels/slices;
- no hidden mutex serialized across unrelated symbols;
- no asynchronous worker reading mutable state without an immutable event boundary.

If you encounter a separate throughput hazard outside the required ownership chain, report it rather than expanding unrelated scope.

---

# COMPLETION AUDIT

Before declaring the work complete, inspect the repository again rather than assuming the refactor removed every old path.

At minimum run:

```bash
gofmt -w <all changed go files>
go test ./...
go test -race ./...
go vet ./...
```

Also search Go source for prohibited patterns, including:

```bash
rg '\}\s*else' -g '*.go'
rg 'math\.IsNaN|math\.IsInf' -g '*.go'
rg 'log\.Fatal|log\.Printf|log\.Println' -g '*.go'
rg 'forwardDelay|DefaultDiscoveryPolicy|AdoptLegacyStore'
```

Audit error-result assignments to `_` manually; not every blank identifier is an error.

Search for the deleted proxy APIs and old public pricing helpers to ensure no compatibility path remains.

Inspect the final constructor signatures. Constructor fan-out should have decreased, not merely moved.

Inspect the final call sites for price/fee economics. A normal consumer should perform one semantic call, not assemble the calculation.

Inspect `Desk`, `Position`, and persistence owners. Their field/method counts and responsibilities should have materially contracted.

Inspect the diff for files that became obsolete and delete them.

A large positive line-count refactor that mostly wraps existing code is a warning sign.

The expected architectural direction is:

**fewer public paths + fewer dependencies per caller + stronger owners + simpler call sites + explicit failures + adaptive statistics.**

Do not stop with tests passing if both the old and new ownership paths remain.

When the requested architecture is fully replaced, tests pass, and the compliance audit is clean, stop.