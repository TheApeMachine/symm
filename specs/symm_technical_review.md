# Technical review of `symm`, with `nomagique` and `datura`

## Review basis

I reviewed the three supplied repository snapshots as one runtime system: `symm` as the application, `nomagique` as its numerical and statistical layer, and `datura` as its data-structure and cognitive-storage layer. I reconstructed 774 files and parsed all 583 Go source files successfully. The supplied snapshots do not include `go.mod`, `go.sum`, `package.json`, TypeScript configuration, or CI definitions, so I could not perform a dependency-aware build, run the full test suites, or run the race detector. Findings below are therefore static source findings, with exact paths and line references from the supplied snapshots.

The order follows the requested emphasis on correctness and performance. It is not a severity or priority ranking.

## Overall assessment

The project has a coherent ambition: a streaming market-data system, signal layer, analytical manifold, planner, broker, UI, and persistence substrate. The central problem is that the runtime model does not enforce the boundaries its domain requires. A market "cut" is represented by a single mutable `*Thesis` shared across independently scheduled actors. In the fixture path, a cut is explicitly reset, measured, analyzed, planned, and applied. In the production actor path, that cut boundary is absent. This makes otherwise careful numerical and trading code operate on state whose temporal identity is not reliable.

The same pattern recurs in the supporting repositories. Many APIs advertise lock-free, immutable, or zero-allocation behavior, but expose mutable backing values, fixed-capacity truncation, ambiguous queue results, or persistence paths that publish memory before durable logging. The best general correction is to make ownership explicit: one serial owner for each mutable state machine, immutable snapshots at subsystem boundaries, and durable transactions that commit in one defined order.

## 1. Runtime correctness and event model

### 1. The production path never creates a new market cut

**What is wrong or could be better.** The only path that calls `Crypto.NextTick`, `Thesis.ResetTick`, and reinstalls a bounded measurement set is `Stack.Observe`, which is used by the fixture-owned execution path. The production wiring instead chains signal → analyzer → planner → crypto actors and passes the same `*Thesis` through every event. `Planner.Decide` appends decisions, and `Crypto.Apply` iterates the complete decision slice each time.

**Reasoning.** In production, `Thesis.Tick` can remain unchanged, per-cut slices are not cleared, and decisions from an earlier event remain eligible for submission on later ticker, book, or trade events. Evidence from different event times is also mixed into one logical decision surface. This is a temporal correctness defect, not merely a synchronization concern.

**Best solution.** Introduce a `CutCoordinator` that owns the tick counter and creates one immutable `Cut` envelope per source sequence. It must reset per-cut state, gather signal results, invoke analyzer and planner once, and hand exactly one finalized command batch to execution. Keep the durable thesis as a consumer of completed cuts rather than the object being mutated through the pipeline.

**Source references.** `symm/stack/boot.go:381-437`; `symm/strategy/planner.go:111-172`; `symm/trader/crypto.go:83-123`.

### 2. Hawkes output is being used as an accidental fan-in barrier

**What is wrong or could be better.** All signals are initialized against the shared thesis, but the analyzer subscribes only to the Hawkes actor topics. Hawkes forwards the thesis only when its own calculation returns at least one measurement. There is no acknowledgement from the other signals and no source-sequence comparison.

**Reasoning.** Analysis can run before slower signals have published their rows, and analysis does not run at all for an event on which Hawkes is not ready. A signal that finishes after the planner has acted updates the same thesis but is not part of the decision that was already submitted.

**Best solution.** Have every signal return a typed `SignalResult{CutID, Source, Measurements, Status}` to the cut coordinator. Finalize a cut only after every interested signal has produced either a result or an explicit skip for that exact `CutID`.

**Source references.** `symm/stack/boot.go:381-403`; `symm/logic/analyzer.go:89-103`; `symm/signal/hawkes/signal.go:89-140`.

### 3. `Thesis` is a shared mutable aggregate without a single owner

**What is wrong or could be better.** Only the measurement slice is consistently guarded by `publish`. Tick, time, forecasts, decisions, hypotheses, categories, resonance, causal output, the incomplete flag, and values stored behind `sync.Map` pointers are mutated without one encompassing ownership rule.

**Reasoning.** `sync.Map` protects map structure, not the fields of the pointed-to `Holding`, graph, cognition, or manifold values. Concurrent reset, publication, analysis, serialization, and execution can observe partial state or produce data races.

**Best solution.** Make the cut coordinator the sole writer of a typed mutable cut builder. Publish an immutable `CutSnapshot` after completion, and require all downstream stages to return new typed results instead of mutating the durable thesis.

**Source references.** `symm/types/thesis.go:41-62`; `symm/types/thesis.go:94-145`; `symm/types/thesis.go:154-232`.

### 4. `CutSnapshot` is shallow and does not provide snapshot isolation

**What is wrong or could be better.** The snapshot copies slices but retains the same `CrossSection` pointer, the same `*Measurement` elements, and nested pointer/map fields inside forecasts, decisions, findings, and hypotheses. It also reads most fields without synchronization.

**Reasoning.** A later mutation can change a supposedly historical snapshot. Concurrent mutation during copying can produce an internally inconsistent snapshot whose fields came from different moments.

**Best solution.** Define immutable wire/value representations for every cut-owned type and deep-copy into those representations under the coordinator ownership boundary. Historical storage and UI publication should accept only that immutable value.

**Source references.** `symm/types/thesis.go:240-256`.

### 5. The actor loop busy-spins and destroys cross-topic ordering

**What is wrong or could be better.** `Actor.Run` continuously calls `handle` through a `default` branch. `handle` scans a Go map and performs nonblocking receives from each topic. Map iteration order is deliberately unspecified, and an idle actor never blocks.

**Reasoning.** Idle CPU consumption scales with actors multiplied by subscriptions. Under load, ticker, book, and trade order is decided by scheduler timing and random map iteration rather than venue/source sequence. That makes stateful signals and execution paths nondeterministic.

**Best solution.** Replace per-topic polling with one blocking actor inbox carrying `Envelope{Topic, SourceSequence, EventTime, Payload}`. One actor goroutine must consume that queue in order and perform fan-out only after the handler finishes.

**Source references.** `symm/types/actor.go:131-171`.

### 6. Subscription delivery can silently discard authoritative messages

**What is wrong or could be better.** `Subscription.Send` retries a hard-coded number of times, sleeps through `utils.Backoff`, logs every retry, returns no result, and exits silently when all retries are exhausted. It does not observe actor cancellation and ignores the configured retry count.

**Reasoning.** A dropped balance, execution, order acknowledgement, or market delta is indistinguishable from successful delivery to the caller. Sleeping and logging inside a websocket callback can also stall the socket reader.

**Best solution.** Make inbox enqueue return a typed result and apply one explicit bounded policy per topic. Authoritative account and order messages must block with context cancellation; replaceable UI/mark updates may coalesce by key and must increment drop/coalesce metrics.

**Source references.** `symm/types/actor.go:37-49`; `symm/cmd/cfg/config.yml:3274-3276`.

### 7. Actor registration and lifecycle are not safe after construction

**What is wrong or could be better.** `subscriptions` and `subscribers` are ordinary maps. `AddRoot`, `Subscribe`, and `Initialize` mutate them without a lock while the run goroutine may be iterating. `Run` can be called repeatedly, closed channels are read without checking `ok`, and there is no waitable shutdown.

**Reasoning.** A concurrent subscription can race map iteration, duplicate `Run` creates multiple consumers, and a closed channel can repeatedly deliver its zero value. Cancellation only requests shutdown; callers cannot know that handlers have stopped.

**Best solution.** Freeze actor topology before start, expose a single `Start` transition, check receive closure, and track the event loop with a `WaitGroup`. `Close` must cancel and wait for the loop to exit.

**Source references.** `symm/types/actor.go:69-179`.

### 8. Fixture synchronization is based on a measurement-count heuristic

**What is wrong or could be better.** `Stack.settle` waits until the length of the measurement slice is unchanged for five polls, with a 500 ms wall-clock deadline.

**Reasoning.** The count can remain unchanged while rows are replaced, a signal is still computing, or an actor message has not yet been consumed. Tests can therefore pass or fail depending on machine scheduling and can validate a partial cut.

**Best solution.** Give each fixture event a cut ID and wait on the same per-signal completion barrier used by production. Remove time-based settling entirely.

**Source references.** `symm/stack/boot.go:466-487`.

### 9. Shutdown leaves active components running against closed dependencies

**What is wrong or could be better.** `Stack.Close` closes Crypto, API, UI hub, tree, and recorder, but does not close signals, analyzer, planner, allocator, desk, instrument, or balance. Their actor goroutines inherit contexts but the assembled stack does not cancel and wait for the whole graph before closing storage and audit resources.

**Reasoning.** Handlers can continue after the tree or recorder is closed, producing misleading errors, lost final records, or races during teardown.

**Best solution.** Make `Stack` own one root context and a lifecycle registry. Stop ingress first, cancel the root, wait for every actor and worker, then close stateful dependencies in reverse construction order.

**Source references.** `symm/stack/boot.go:520-546`; `symm/logic/analyzer.go:113-127`; `symm/strategy/planner.go:106-109`.

### 10. Startup readiness polling can wait forever and can report readiness too early

**What is wrong or could be better.** `Stage.Initialize` polls each reporter every 10 ms with no context or deadline. Separately, `Live.Initialize` marks the websocket `READY` immediately after `Connect`, even though authentication and resubscription happen asynchronously in callbacks.

**Reasoning.** Boot can hang permanently on a reporter that never transitions, or it can proceed before a private websocket is authenticated and before required subscriptions are active.

**Best solution.** Use a context-aware readiness future returned by each component. The live websocket future should resolve only after connection, authentication when required, and acknowledgement of every required subscription.

**Source references.** `symm/system/stage.go:89-123`; `symm/kraken/websocket/live.go:189-243`; `symm/kraken/websocket/live.go:294-308`.

### 11. Websocket receive callbacks perform blocking application work

**What is wrong or could be better.** The SDK receive callback parses the same JSON field several times, mutates increment state, constructs typed entities, and calls `Subscription.Send`, whose retry path sleeps and logs.

**Reasoning.** Any slow downstream actor can delay the websocket reader. That increases the chance of book gaps, heartbeats being delayed, and exchange-side disconnects precisely during bursts.

**Best solution.** Decode the envelope once in the callback and perform one nonblocking handoff into a dedicated ingress owner. Parsing, normalization, validation, and downstream delivery should run off the socket reader.

**Source references.** `symm/kraken/websocket/live.go:111-151`; `symm/types/actor.go:37-49`.

### 12. Configuration is globally mutable while most components snapshot values once

**What is wrong or could be better.** Components call Viper directly during construction and operation, while `viper.WatchConfig` is enabled without an atomic reconfiguration protocol. Some values are copied into fields, while other code reads Viper on each call.

**Reasoning.** A file change can create a hybrid runtime in which different subsystems use different generations of configuration. Global access also makes tests order-dependent.

**Best solution.** Load, normalize, and validate one immutable `Config` at startup and inject typed sub-configs into constructors. Remove live watching until a complete generation can be validated and swapped atomically.

**Source references.** `symm/cmd/root.go:3501-3554`; `symm/broker/desk.go:31-47`; `symm/broker/instrument.go:1680-1691`.

## 2. Broker, allocation, and execution correctness

### 13. Balance snapshots merge into old state instead of replacing it

**What is wrong or could be better.** A snapshot stores every incoming asset into the existing `sync.Map` but never removes assets absent from the snapshot. It also returns before recording the snapshot sequence. Incremental updates accept duplicate sequence numbers and do not detect gaps.

**Reasoning.** An asset removed from the exchange snapshot can remain as stale cash or inventory. A missed update is not detected, so later state can be accepted even though the local wallet is no longer authoritative.

**Best solution.** Build a complete new balance map from every snapshot, validate and store its sequence, then atomically replace the current immutable wallet state. Require exact next-sequence continuity for updates and request a fresh snapshot on any duplicate, regression, or gap.

**Source references.** `symm/broker/balance.go:93-131`.

### 14. `sync.Map` values are mutated in place without synchronization

**What is wrong or could be better.** Balance, desk, position, and price paths load `*Holding`, `*Position`, and Kraken data pointers and change their fields directly. Concurrent ticker, balance, order, execution, planner, UI, and serialization handlers can access the same object.

**Reasoning.** The concurrent map protects only lookup and replacement. It does not make `holding.Qty`, `Status`, fees, marks, P&L, or timestamps safe. Readers can observe torn logical state even where individual machine-word accesses happen to be atomic.

**Best solution.** Give the broker one serial event loop that owns plain typed maps. Publish copied immutable position and wallet snapshots to readers.

**Source references.** `symm/broker/balance_frame.go:93-176`; `symm/broker/position.go:73-156`; `symm/broker/price.go:445-548`.

### 15. The advertised cash reservation model is not implemented

**What is wrong or could be better.** Comments refer to a live claim ledger, but `Balance.Available` simply subtracts one requested amount from the current exchange cash. The UI always reports `available == balance` and `reserved == 0`.

**Reasoning.** Concurrent or batched entry decisions can each pass against the same money. Order submission then races the exchange balance update, so the application can overcommit capital while presenting no reservation to the UI.

**Best solution.** Implement a reservation ledger inside the balance owner with atomic `Reserve`, `Commit`, and `Release` transitions keyed by intent ID. Derive UI total, available, and reserved values from that same ledger.

**Source references.** `symm/broker/balance.go:228-297`; `symm/broker/balance_frame.go:32-50`.

### 16. Slot checks, cash checks, and order submission are separate operations

**What is wrong or could be better.** `Desk.HasSlot`, `Allocator.Allocate`, `Balance.Available`, `Desk.Buy`, and `Position.Enter` run independently. `Desk.Buy` does not enforce a slot, and the `opportunity` argument is accepted but unused.

**Reasoning.** Two entry paths can both see a free slot and sufficient cash, then both submit. A lower layer cannot guarantee the invariants assumed by the planner.

**Best solution.** Create one broker command `ReserveAndSubmitEntry` handled by the desk owner. In one transition it must reserve a slot and exact cash, create the intent, and submit the order; failure must release both reservations.

**Source references.** `symm/broker/desk.go:266-326`; `symm/broker/position.go:186-235`.

### 17. Every order and execution frame is decoded once per open position

**What is wrong or could be better.** `Desk.onExecutions` and `Desk.onOrder` broadcast the raw frame to every position. Each position reparses it and scans the contained rows for its order ID.

**Reasoning.** The cost is proportional to positions × frames and creates repeated allocation and validation on an account hot path. It also makes routing dependent on mutable position-local IDs.

**Best solution.** Decode each account frame once in the desk event loop and route rows through indexes keyed by request ID and order ID.

**Source references.** `symm/broker/desk.go:170-190`; `symm/broker/position.go:97-156`.

### 18. Executions can be ignored when they arrive before the order acknowledgement

**What is wrong or could be better.** A position accepts an execution only when `data.OrderID == position.orderID`. `orderID` is set by `OrderAck`; before that it is empty. There is no client-order identifier correlation, buffering, REST reconciliation, execution-ID deduplication, or sequence tracking.

**Reasoning.** A fast market fill or reordered private stream can be dropped permanently. Replayed executions can also be applied more than once.

**Best solution.** Use an idempotent order state machine keyed first by client request ID and then by exchange order ID. Buffer unmatched events by correlation key, deduplicate by execution ID, and reconcile unresolved intents from the venue ledger.

**Source references.** `symm/broker/position.go:97-156`; `symm/kraken/websocket/conn.go:235-370`.

### 19. Exit validation can submit more inventory than is sellable

**What is wrong or could be better.** `Position.Exit` checks that wallet availability is positive, then creates the sell order for `holding.Qty`. It never caps or rejects when `holding.Qty > available`.

**Reasoning.** A stale holding or partially reserved wallet can create an exchange rejection or sell inventory belonging to another intent.

**Best solution.** Compute the executable exit quantity from the broker-owned inventory ledger, subtract active sell reservations, floor it to the instrument increment, and submit exactly that quantity.

**Source references.** `symm/broker/position.go:241-307`.

### 20. Inventory quantity has two writers with different event timing

**What is wrong or could be better.** `Price.fillExit` subtracts execution quantity from `holding.Qty`, while its comment says inventory is owned by `Balance.syncWallet`; balance updates also overwrite quantity from the exchange wallet.

**Reasoning.** Depending on arrival order, a fill can be subtracted and then overwritten, or a fresh balance can be followed by another subtraction. P&L and position closure can therefore use a transient or double-adjusted quantity.

**Best solution.** Make fills the sole source of order economics and the wallet snapshot the sole source of available inventory. Store them in separate fields and derive position quantity through one reconciliation function in the broker owner.

**Source references.** `symm/broker/price.go:500-548`; `symm/broker/balance_frame.go:93-176`.

### 21. Partial-fill economics are overwritten rather than aggregated

**What is wrong or could be better.** Entry and exit price and fee fields are assigned from each execution row. The code assumes exchange `AvgPrice` and `FeeUsdEquiv` already express the correct lifetime aggregate, but that contract is not enforced and realized P&L is not represented separately.

**Reasoning.** Multiple fills, partial exits, fees in different currencies, and reopenings can overwrite prior economics or mix remaining-lot and realized values.

**Best solution.** Maintain a fill ledger per intent and derive weighted entry cost, remaining cost basis, cumulative fees, realized P&L, and unrealized P&L from immutable fills.

**Source references.** `symm/broker/price.go:445-548`; `symm/types/holding.go`.

### 22. Quantity sizing can silently omit fees and has an arbitrary termination bound

**What is wrong or could be better.** `Price.Quantity` continues with ask-only unit cost when `Fraction` returns an error. It then retries budget fitting at most eight times.

**Reasoning.** Missing fee data can size an order above budget. The fixed iteration count has no relation to quantity precision or increment count, so a valid executable quantity can be rejected or an arithmetic defect can be hidden behind a generic convergence error.

**Best solution.** Treat fee lookup as mandatory and calculate quantity directly on the integer quantity-increment lattice: compute an upper bound, then use monotone integer binary search for the largest quantity whose exact taker cost fits the budget.

**Source references.** `symm/broker/price.go:756-897`.

### 23. The allocator reuses the same wallet slice for every entry

**What is wrong or could be better.** One `slice = cash * maxFraction` is calculated before the decision loop. Every entry is sized from that same slice, and every availability check sees the unchanged exchange balance because no reservation is created.

**Reasoning.** A batch of several accepted entries can allocate several times the intended fraction and collectively exceed available cash.

**Best solution.** Begin allocation with a transaction-local available budget from the reservation ledger. Reserve each accepted decision’s exact taker cost immediately and decrement the remaining budget before processing the next decision.

**Source references.** `symm/strategy/allocator.go:65-188`.

### 24. Allocator contracts for rotation and risk are not honored

**What is wrong or could be better.** Rotation admission writes a proposed freed-capital notional, but the allocator overwrites `ProposedNotional` using the normal wallet slice. `AllocationHaircut` is converted directly to a decimal without validating that it is finite and within `[0,1]`. A missing `QtyMin` or reference price returns from the whole batch.

**Reasoning.** The replacement trade is not sized from the capital it is supposed to release, malformed risk can create nonsensical budgets, and one bad instrument prevents later independent decisions from being evaluated.

**Best solution.** Validate every decision into a typed allocation request, use its proposed funding source and amount, constrain haircut to `[0,1]`, and record a per-decision rejection while continuing the batch.

**Source references.** `symm/strategy/admit.go:47-72`; `symm/strategy/allocator.go:65-188`.

### 25. Rotation submits the replacement entry before the incumbent exit

**What is wrong or could be better.** `Rotate.Commit` collects exits and appends them after the existing entry decisions. `Crypto.Apply` iterates in slice order, so it attempts the challenger buy before selling the displaced holding.

**Reasoning.** The capital assumed to be freed is not available yet, the slot may still be occupied, and a failed exit can leave both positions or no coherent rotation state.

**Best solution.** Represent rotation as a persisted saga: submit the incumbent exit, wait for confirmed fill and wallet/slot release, calculate actual proceeds, and only then create and submit the replacement entry.

**Source references.** `symm/strategy/rotate.go:188-245`; `symm/trader/crypto.go:100-116`.

### 26. Displacement state is mutated before the candidate is durably accepted

**What is wrong or could be better.** The arbiter marks an incumbent as displaced and mutates candidate metadata during selection, before allocation and order submission have succeeded. Later stages may convert the candidate to `ActionNothing`.

**Reasoning.** Lifecycle and position state can say that an incumbent was displaced even though no executable replacement exists.

**Best solution.** Keep selection output immutable and provisional. Apply displacement state only when the rotation saga commits its exit intent.

**Source references.** `symm/strategy/arbiter.go:122-173`; `symm/strategy/rotate.go:188-245`.

### 27. Status values and transitions are not centralized

**What is wrong or could be better.** The status vocabulary contains both `CANCELLED` and `CANCELED`, and components assign status fields directly. Unknown execution types are cast into arbitrary `types.Status` strings.

**Reasoning.** Equivalent states can compare unequal, and invalid transitions are accepted silently. Cleanup logic that checks one spelling may retain or remove the wrong object.

**Best solution.** Define one canonical status enum and one transition table. All state changes must go through a transition function that rejects unknown states and illegal edges.

**Source references.** `symm/types/status.go`; `symm/broker/position.go:141-156`; `symm/broker/desk.go:196-224`.

### 28. Instrument metadata is exposed as mutable shared pointers

**What is wrong or could be better.** `Remember` stores a caller-owned pointer and `Pair` returns the stored pointer. Subscription symbols are also written and read without synchronization, and subscription work is performed serially with sleeps inside initialization.

**Reasoning.** A caller can mutate exchange precision metadata after validation. Reconnect can read a partially updated symbol list, while large universes lengthen boot unnecessarily.

**Best solution.** Build a validated immutable instrument snapshot, copy on ingress and return by value. Let one subscription coordinator own the symbol generation and issue paced batches asynchronously while reporting completion.

**Source references.** `symm/broker/instrument.go:1803-1949`.

### 29. Decimal ownership is ambiguous at API boundaries

**What is wrong or could be better.** Several getters return pointers stored in caches or domain objects, and decisions and holdings sometimes receive the same decimal pointer rather than a copy. Arithmetic helpers also accept mutable decimal pointers without nil/zero-divisor contracts.

**Reasoning.** A caller can accidentally mutate cached market or wallet data, and aliasing makes it difficult to prove which state a calculation used.

**Best solution.** Introduce a project-owned immutable `Money`/`Quantity` value that stores integer units plus scale. Convert to the SDK decimal only at the exchange adapter boundary and return values, never internal pointers.

**Source references.** `symm/broker/balance.go:134-161`; `symm/broker/price.go:728-754`; `symm/strategy/allocator.go:177-184`.

### 30. Mark routing has two competing designs and one is unused

**What is wrong or could be better.** `Price` exposes `RouteMarks` and invokes `onMark` after ticker storage, but no caller registers it. `Desk.onTicker` separately scans ticker rows, looks up positions, and marks them.

**Reasoning.** The unused callback creates a false assumption that price updates fan out centrally, while the actual path performs repeated desk lookups and is easy to bypass in another caller of `TickerAck`.

**Best solution.** Make `Price` a pure cache and route typed ticker rows only through the desk owner, which updates the cache and affected position in one ordered handler. Remove `RouteMarks`.

**Source references.** `symm/broker/price.go:82-108`; `symm/broker/desk.go:107-128`.

## 3. Persistence, observability, UI, and operational performance

### 31. The audit recorder can lose a write during close and hides asynchronous I/O failure

**What is wrong or could be better.** `Write` checks context, marshals, and then pushes. `Close` can cancel and drain between the check and the push, leaving a row behind after the drain exits. Writer and flush errors are only logged; `Close` returns only the file close result. `Close` is not safe to call concurrently.

**Reasoning.** Audit data can be reported as accepted but never reach disk, and callers cannot determine that the recorder failed earlier. This is especially damaging because the recorder is intended to diagnose freezes and sequence faults.

**Best solution.** Add an atomic closing gate and in-flight producer count. Stop new writers, wait for accepted producers, drain to quiescence, retain the first asynchronous error, flush and sync, and return the combined result from an idempotent `Close`.

**Source references.** `symm/audit/writer.go:89-223`.

### 32. Audit overflow reporting is delayed by continuous traffic

**What is wrong or could be better.** Dropped-count records are generated only when `Pop` returns empty. Under a sustained full queue, the consumer may remain nonempty and not record the overflow until much later.

**Reasoning.** The diagnostic stream can omit the period in which it was least trustworthy, and the producer hot path logs an error for every rejected row.

**Best solution.** Maintain overflow as a counter metric and have the consumer emit a coalesced overflow record on a bounded interval independent of queue emptiness.

**Source references.** `symm/audit/writer.go:131-201`.

### 33. Thesis checkpointing is not a fully durable atomic replace

**What is wrong or could be better.** The file is written through a temp file and renamed, but the directory is not synced after rename. More importantly, `MarshalJSON` can run while the shared thesis and pointed-to map values are being mutated.

**Reasoning.** A power loss can lose the directory entry on filesystems requiring directory sync, and a successful checkpoint can contain a cross-generation state.

**Best solution.** Serialize an immutable completed-cut snapshot, fsync the temp file, rename it, and fsync the containing directory before reporting success.

**Source references.** `symm/types/thesis.go:259-334`; `symm/types/thesis.go:336-400`.

### 34. One slow websocket client blocks the hub’s only drain loop

**What is wrong or could be better.** `Hub.drain` retains a frame and then writes it synchronously. The hub has one atomic client pointer and one global write mutex. A newly connected client replaces the pointer while the old connection remains alive.

**Reasoning.** A slow or stalled client stops consumption of the shared UI channel, causing upstream nonblocking publishers to drop frames. Only the newest client receives updates, but older clients continue consuming resources.

**Best solution.** Give each client a bounded latest-by-key writer queue managed by the hub event loop. Register multiple clients explicitly and disconnect a client whose queue cannot keep up.

**Source references.** `symm/ui/hub.go:30-109`; `symm/ui/hub.go:141-239`.

### 35. Replay can regress a newly connected client and cached coverage is incomplete

**What is wrong or could be better.** Replay iterates cached keys while the live drain can update and write newer frames. There is no frame generation number. `cacheKeys` omits several actively painted streams such as causal, resonance, manifold, cognition, and diagnostics.

**Reasoning.** A client can receive a current live value followed by an older replayed value, and reconnect state is incomplete for omitted streams.

**Best solution.** Assign every hub update a monotonically increasing generation. Snapshot the complete latest-by-key cache and its generation when a client joins, replay that snapshot, then deliver only later generations.

**Source references.** `symm/ui/hub.go:19-22`; `symm/ui/hub.go:113-177`.

### 36. UI loss is silent and immediate retry branches do not improve delivery

**What is wrong or could be better.** Many publishers use a nonblocking send and discard on a full channel. Balance and instrument publication immediately repeat the same nonblocking send, without yielding or freeing capacity.

**Reasoning.** The second attempt has effectively the same queue state and does not establish latest-value semantics. Important state can disappear with no metric or recovery path.

**Best solution.** Publish through a typed hub API that coalesces replaceable state by key and tracks dropped nonreplaceable events. Remove direct channel access from subsystems.

**Source references.** `symm/broker/balance.go:76-91`; `symm/broker/instrument.go:1750-1787`; `symm/trader/crypto.go:159-163`.

### 37. The frontend performs unbounded work per websocket frame

**What is wrong or could be better.** The worker parses an arbitrary object and structured-clones the whole frame to the main thread. `attach` synchronously runs every registered painter for every top-level key. There is no runtime schema validation, animation-frame coalescing, or latest-by-key replacement.

**Reasoning.** A burst can create a long main-thread queue, render obsolete intermediate states, and repeatedly project/copy history. Malformed fields fail deep inside painters rather than at the boundary.

**Best solution.** Validate frames in the worker against a versioned schema, coalesce replaceable keys, and post one typed batch per animation frame. The main thread should render one latest snapshot per frame.

**Source references.** `symm/frontend/src/providers/ws-worker.ts:110-140`; `symm/frontend/src/providers/ws-stores.ts:266-315`.

### 38. `FrameHistory` bounds each series but not entity cardinality

**What is wrong or could be better.** The outer maps retain every identity ever seen. Focus retention is applied only when a row for that entity arrives, timestamps are compared as strings, and `values` flattens map iteration order rather than globally sorting events.

**Reasoning.** Long-running universes can grow the map indefinitely. Changing focus does not immediately shrink the previously focused series. Noncanonical timestamps can sort incorrectly, and combined charts can receive nonchronological data.

**Best solution.** Store numeric epoch timestamps, impose an LRU bound on entities per stream, prune immediately on focus change, and merge series into one globally time-ordered projection.

**Source references.** `symm/frontend/src/providers/frame-history.ts:57-170`.

### 39. The simulator is global process state

**What is wrong or could be better.** The websocket simulator uses singleton initialization and shared mutable state across callers. Timing behavior relies on real sleeps rather than an injected clock.

**Reasoning.** Independent stacks and tests can influence one another, and timing tests become slow and nondeterministic.

**Best solution.** Construct one simulator per stack with an injected deterministic clock, seed, and scheduler. All delays must wait through the stack context.

**Source references.** `symm/kraken/websocket/simulator.go`.

### 40. Paper transport shells out to an external executable

**What is wrong or could be better.** The paper path invokes a `kraken` command through `exec.Command`. Its behavior depends on PATH, CLI version, environment, and subprocess timing, and the command is not bound to the stack context.

**Reasoning.** The supposedly deterministic local execution path can fail for machine-specific reasons and leave subprocesses running during cancellation.

**Best solution.** Implement the paper exchange as an in-process matching engine behind the same transport interface, using the injected simulator clock and order book.

**Source references.** `symm/kraken/websocket/paper.go`.

### 41. Library code mixes error construction, global logging, and control flow

**What is wrong or could be better.** Many low-level helpers call `errnie.Error` while also returning the error; others only log and continue. Hot paths log expected not-ready conditions or queue pressure, while some loss paths return nothing.

**Reasoning.** Callers cannot decide policy cleanly, the same fault can be logged several times, and logging cost contaminates latency-sensitive code.

**Best solution.** Return typed errors from libraries without side effects. Log once at the process boundary with structured context and expose counters for expected saturation or not-ready states.

**Source references.** `symm/types/actor.go:37-49`; `symm/audit/writer.go:89-120`; `nomagique/adaptive/readiness.go`.

### 42. Pervasive `any` and unversioned JSON weaken cross-component contracts

**What is wrong or could be better.** Actor handlers accept `any`, many frames are assembled as `map[string]any`, and the frontend accepts `Record<string, unknown>`. Type assertions are spread through handlers and wire names double as routing identifiers.

**Reasoning.** A schema change becomes a runtime panic or silent ignored key. There is no negotiated protocol version or exhaustive handling across backend and frontend.

**Best solution.** Define versioned typed envelopes and generate Go and TypeScript wire types from one schema. Actor handlers should consume concrete types and the UI should reject incompatible versions at ingress.

**Source references.** `symm/types/actor.go:54-76`; `symm/ui/hub.go:113-138`; `symm/frontend/src/providers/ws-worker.ts:110-140`.

## 4. `nomagique` numerical and statistical layer

### 43. `Compression` uses data values as initialization state

**What is wrong or could be better.** A stored baseline `<= 0` is treated as uninitialized. Zero or negative samples therefore keep resetting the baseline. Output `Count` is always one regardless of observations.

**Reasoning.** The result depends on the sign and unit of the series rather than the intended compression definition, and readiness metadata is false.

**Best solution.** Store an explicit per-series state containing `initialized`, baseline, and count. Validate the domain expected by compression and update count on every accepted observation.

**Source references.** `nomagique/adaptive/compression.go:6-71`.

### 44. Several adaptive stages report readiness before the statistic is identifiable

**What is wrong or could be better.** `Delta` defines the first observation as a maximal normalized change of one, and sample variance reports ready zero variance after one sample. Other stages use different first-sample semantics.

**Reasoning.** Downstream code cannot interpret `Ready` consistently. A fabricated first delta can trigger a classifier, while one-sample variance understates uncertainty.

**Best solution.** Adopt one library-wide readiness contract: a stage is ready only when the mathematical quantity is identifiable from observed data. Delta requires two observations; sample variance requires two observations.

**Source references.** `nomagique/adaptive/delta.go:34-76`; `nomagique/adaptive/variance.go:34-69`.

### 45. Running min/max normalization has permanent outlier memory

**What is wrong or could be better.** `Delta`, `Momentum`, `ZScore`, and fractional differencing derive adaptation rates from the all-time observed span.

**Reasoning.** One extreme early value permanently compresses all future movement, making scores depend on process age and historical units rather than the current regime.

**Best solution.** Use an event-time decayed robust location and scale estimator, such as exponentially weighted median/MAD approximations, and normalize against that bounded-memory state.

**Source references.** `nomagique/adaptive/delta.go`; `nomagique/adaptive/momentum.go`; `nomagique/adaptive/zscore.go`; `nomagique/adaptive/fracdiff.go`.

### 46. `LogReturn` can retain unbounded history and unbounded series keys

**What is wrong or could be better.** When `LongWindow` is zero, `trim` returns the complete slice forever. The map of series has no eviction. Computing a fixed-lag return needs only the lag window.

**Reasoning.** Memory grows with every sample and every symbol/key ever observed, despite the operation having bounded state requirements.

**Best solution.** Use a fixed-size ring of `ReturnLag + 1` observations per series and put the series states behind an explicit cardinality-bounded registry.

**Source references.** `nomagique/adaptive/log_return.go:30-157`.

### 47. EMA measurement allocates channels and slices for an O(1) recurrence

**What is wrong or could be better.** Every call converts samples to a channel, creates background-context computation, drains an output channel into a slice, and takes the last value.

**Reasoning.** This introduces allocation, scheduling, and channel overhead into a basic streaming statistic, especially when called with one sample at a time.

**Best solution.** Store the EMA value and initialization flag directly and apply the recurrence in `Measure` with no channels or intermediate slices.

**Source references.** `nomagique/adaptive/ema.go:54-72`.

### 48. Fractional differencing is unit-dependent and changes its kernel over time

**What is wrong or could be better.** The order, weight threshold, and maximum lag are derived from the raw sample span. Rescaling an otherwise identical series changes the filter. Weights are rebuilt when the adaptive order moves, so historical samples are reinterpreted under a new kernel.

**Reasoning.** The output is not a stable fractional difference and is difficult to compare across assets or units. Large spans also drive longer work up to the fixed cap.

**Best solution.** Use a configured fractional order and tolerance, precompute one bounded weight vector, and apply it to a fixed ring. Normalize the input separately when scale invariance is required.

**Source references.** `nomagique/adaptive/fracdiff.go:68-345`.

### 49. `Accumulator` can become non-finite after accepting finite inputs

**What is wrong or could be better.** Inputs are checked individually, but repeated finite addition can overflow and the output is not validated. Straight summation also accumulates avoidable floating-point error.

**Reasoning.** A stage can emit infinity even though its contract rejects non-finite values, and long-running sums lose low-order contributions.

**Best solution.** Use compensated summation, validate the updated result, and return a typed overflow error without committing the invalid state.

**Source references.** `nomagique/adaptive/accumulator.go:15-43`; `nomagique/adaptive/readiness.go`.

### 50. `RLS.Measure` predicts after training on the same target

**What is wrong or could be better.** `Measure` calls `observe(features, target)` and then predicts with those features. A separate `Predict` method already exists for prior prediction.

**Reasoning.** Using `Measure.Value` as a forecast leaks the outcome into the prediction and produces optimistically biased evaluation and calibration.

**Best solution.** Make `Observe` update state and return update diagnostics only. Require all forecasts to use `Predict` before the corresponding target is observed.

**Source references.** `nomagique/learning/rls.go:65-110`.

### 51. RLS allocates quadratic output and temporary work on every sample

**What is wrong or could be better.** Each output copies beta, flattens the full covariance matrix, and also copies its diagonal. The update allocates design and matrix-vector work buffers.

**Reasoning.** For feature dimension `d`, the public output copies O(d²) data per tick even when callers need only the prediction. This can dominate the actual RLS arithmetic.

**Best solution.** Implement square-root RLS with a reusable workspace. Return prediction and scalar diagnostics on the hot path, and expose an explicit snapshot method for coefficient/covariance inspection.

**Source references.** `nomagique/learning/rls.go:65-176`; `nomagique/learning/rls.go:241-327`.

### 52. Covariance repair does not guarantee a valid covariance matrix

**What is wrong or could be better.** The implementation symmetrizes entries and floors each diagonal, but that does not ensure positive semidefiniteness. On failure it can reset covariance while retaining coefficients.

**Reasoning.** A matrix can have positive diagonals and still be indefinite. Retaining beta while replacing uncertainty with the prior produces internally inconsistent state.

**Best solution.** Use a square-root covariance representation updated by numerically stable orthogonal transformations. On unrecoverable failure, reset coefficients and covariance together and surface the reset.

**Source references.** `nomagique/learning/rls.go:184-327`.

### 53. The forecast calibrator treats zero residual span as an error

**What is wrong or could be better.** Repeated equal residuals produce `span == 0` and return a validation error. The same code uses all-time min/max residuals and a target scale that can inherit sign.

**Reasoning.** Perfectly stable residuals are a valid zero-surprise condition, not invalid data. All-time extremes again make adaptation age-dependent, and signed scale can invert a normalized error.

**Best solution.** Track residual mean and variance online, treat zero variance as zero standardized surprise, and calibrate positive magnitudes in log-ratio space.

**Source references.** `nomagique/learning/forecast.go:47-118`.

### 54. `sameTickSize` accepts materially different price lattices

**What is wrong or could be better.** Two ticks are considered equal when both ratios round to one. Values such as `1.0` and `1.49` satisfy that condition even though their executable lattices are different.

**Reasoning.** The book can retain levels encoded on an obsolete lattice and then interpret them with a new tick size, corrupting touch, spread, and imbalance.

**Best solution.** Represent tick size as an exact decimal/rational and compare its normalized integer numerator and scale exactly.

**Source references.** `nomagique/algorithm/book/flow/book.go:79-89`.

### 55. Book updates are non-transactional and repeatedly scan the entire book

**What is wrong or could be better.** `Apply` mutates levels as it validates each row, so a later invalid row leaves earlier changes applied. Touch detection scans the map per level; best price, depth, and length scan again; weighted depth collects all ticks and insertion-sorts them.

**Reasoning.** A rejected update can still alter state. Work approaches O(U×N + N²) for an update of U levels and a book of N levels, and shared scratch makes concurrent reads unsafe.

**Best solution.** Use a single-owner ordered integer-tick book with cached best/depth aggregates. Validate the complete batch first, then apply it atomically and traverse the required top levels directly.

**Source references.** `nomagique/algorithm/book/flow/book.go:91-410`.

### 56. Invalid side values silently select the ask book

**What is wrong or could be better.** The side helper returns bids only for the exact bid byte and treats every other value as ask.

**Reasoning.** Corrupt or newly introduced side values mutate ask state rather than being rejected, making data corruption hard to detect.

**Best solution.** Use a typed side enum and return a validation error for any value other than bid or ask before touching state.

**Source references.** `nomagique/algorithm/book/flow/book.go:198-204`.

### 57. Book-flow sampling is neither concurrency-safe nor transactionally consistent

**What is wrong or could be better.** Window creation uses load-then-store on `sync.Map`, and each `Window` is mutated without locking. Bid updates are committed before ask validation. History and observation count are updated before a possible `FlatDepth` error. `flatOK` is set true before the result is known.

**Reasoning.** Concurrent first use can create two windows and lose one. An invalid frame can partially change the book and histories, and readiness metadata can claim a valid flat feature that was never computed.

**Best solution.** Assign each symbol to one serial sampler owner. Validate and apply both sides in one atomic book transaction, then compute and append one complete feature observation.

**Source references.** `nomagique/algorithm/book/flow/sample.go:155-250`.

### 58. Trade-before-book input is reported ready with zero market context

**What is wrong or could be better.** `features` returns `ready=true` when weighted history is empty, even if the only event was a trade and mid, spread, and touch depth are all zero. Any side other than exact `sell` is treated as a buy. Trade smoothing becomes a lifetime average as the count grows.

**Reasoning.** Downstream equations can consume a nominally ready but structurally invalid sample, malformed sides create positive pressure, and the pressure stops adapting to regime changes.

**Best solution.** Require a valid two-sided book before readiness, validate side through the typed enum, and use event-time exponential decay for trade pressure.

**Source references.** `nomagique/algorithm/book/flow/sample.go:116-143`; `nomagique/algorithm/book/flow/sample.go:252-303`.

### 59. Book-flow output copies three complete histories per event

**What is wrong or could be better.** Every feature call clones weighted, level-1, and flat history slices before invoking equations.

**Reasoning.** The hot path performs O(window) copying and allocation for each update, despite most calculations needing aggregates or read-only traversal.

**Best solution.** Pass a read-only ring view or precomputed rolling statistics into the equation layer and snapshot full history only for diagnostics.

**Source references.** `nomagique/algorithm/book/flow/sample.go:273-303`.

### 60. Hawkes optimization is coupled to the streaming signal path

**What is wrong or could be better.** The symm Hawkes signal holds its mutex while calling the process measurement. The estimator performs multi-start constrained likelihood optimization, and projection paths can refit restricted models repeatedly.

**Reasoning.** Market ingestion latency becomes dependent on optimizer convergence and retained history. One symbol’s fit stalls all symbols protected by the signal mutex.

**Best solution.** Keep ingestion to append-and-evaluate using the latest retained parameters. Fit unrestricted and restricted models asynchronously per symbol on immutable windows, then atomically publish a new parameter epoch.

**Source references.** `symm/signal/hawkes/signal.go:174-235`; `nomagique/hawkes/estimator.go`; `nomagique/hawkes/optimize.go`.

## 5. `datura` queues, tree, cognition, and persistence

### 61. The MPMC queue cannot distinguish an empty queue from a queued zero value

**What is wrong or could be better.** `Pop` returns the zero value of `T` when empty, while `Push` accepts any `T`. `Slice` pops exactly `count` times and pushes each result even when the source contains fewer elements.

**Reasoning.** Zero is valid for many generic types. Consumers cannot tell empty from data, and slicing beyond length manufactures zero-valued elements.

**Best solution.** Change dequeue to `Pop() (T, bool)` and stop slicing when `ok` is false. Keep queue occupancy semantics independent of payload value.

**Source references.** `datura/structure/mpmc.go:148-180`; `datura/structure/mpmc.go:265-282`.

### 62. MPMC close does not close the queue

**What is wrong or could be better.** `Close` only cancels a derived context, but `Push` and `Pop` never check that context or a closed flag. Producers can continue to enqueue after close.

**Reasoning.** Callers cannot establish a terminal boundary, and resources using the queue can accept work after their consumer has exited.

**Best solution.** Add an atomic closed state checked by enqueue, expose a drain-aware close protocol, and make post-close enqueue return a distinct closed result.

**Source references.** `datura/structure/mpmc.go:148-180`; `datura/structure/mpmc.go:377-389`.

### 63. Queue navigation and merge operations bypass the concurrency model

**What is wrong or could be better.** The same public ring type supports live MPMC operations plus navigators, merge, adopt, and direct slot access. Negative `Select` converts to unsigned sequence arithmetic. Several operations require quiescence only by comment.

**Reasoning.** A caller can corrupt sequence/slot invariants while producers and consumers are active. The API gives no enforceable way to prove quiescence.

**Best solution.** Separate the live queue from an offline immutable snapshot type. Remove navigator mutation and merge/adopt operations from the concurrent queue API.

**Source references.** `datura/structure/mpmc.go:182-264`; `datura/structure/mpmc.go:391 onward`.

### 64. SPSC drop-oldest makes the producer execute consumer work

**What is wrong or could be better.** When full, `Push` calls `Pop`. The type’s contract says exactly one producer and one consumer, so the producer becomes a second consumer. The SPSC queue also has the same zero/empty and overlong-slice ambiguity, and `Close` drains but does not prevent later pushes.

**Reasoning.** Producer and consumer can race tail and slot restoration in a design that was justified by single-consumer ownership.

**Best solution.** Use a sequence-based SPSC overwrite protocol in which the producer advances an eviction cursor without executing consumer dequeue logic, and return `(value, ok)` from pop.

**Source references.** `datura/structure/spsc.go:68-136`; `datura/structure/spsc.go:258-319`.

### 65. `NewTree` hides initialization failure and can dereference a failed store

**What is wrong or could be better.** `NewTree` returns only `*Tree`. Persistent-store construction is wrapped in `guardValue`; the next expression calls `tree.persist.Replay` even if construction returned nil and recorded an error.

**Reasoning.** Callers receive a usable-looking tree instead of an initialization error, and a failed persistent-store construction can panic before the error is observable.

**Best solution.** Change construction to `NewTree(...) (*Tree, error)` and return immediately on store or replay failure before publishing the tree.

**Source references.** `datura/dmt/tree.go:86-118`; `datura/dmt/guard.go:35-49`.

### 66. Tree read APIs expose backing byte slices

**What is wrong or could be better.** Radix values and keys are passed to callers through `Get`, `WalkPrefix`, forest iteration, and related APIs without an ownership contract or copy.

**Reasoning.** A caller can mutate bytes referenced by an otherwise immutable radix root, invalidating snapshots shared by concurrent readers and replicas.

**Best solution.** Store immutable byte strings internally and copy into caller-owned buffers at public boundaries. Provide explicit borrowed views only inside a scoped callback that cannot retain them.

**Source references.** `datura/dmt/tree.go:167-180`; `datura/dmt/tree.go:329 onward`; `datura/dmt/forest.go:271-286`.

### 67. `WalkPrefix` does not enforce the prefix after seeking

**What is wrong or could be better.** Unlike `Seek`, `WalkPrefix` seeks to the prefix and then invokes the callback for every remaining iterator entry without breaking when keys stop sharing the prefix.

**Reasoning.** A caller asking for one namespace can receive unrelated later namespaces and mutate or decay data outside its intended scope.

**Best solution.** Check `bytes.HasPrefix(key, prefix)` in the iteration loop and stop at the first nonmatching key.

**Source references.** `datura/dmt/tree.go:167-180`.

### 68. Latency sampling produces a biased average

**What is wrong or could be better.** Operation count is incremented separately from sampled duration, and the average scales sampled time by a fixed sampling factor over all operations. Boundary races can also cause adjacent operations to make inconsistent sample decisions.

**Reasoning.** The “fastest tree” selector can compare distorted values, including zero for a tree with no measured samples, and route reads based on instrumentation artifacts.

**Best solution.** Track sampled operation count independently and compute sampled total divided by sampled count. Exclude trees without a completed sample from latency-based selection.

**Source references.** `datura/dmt/tree.go:50-85`; `datura/dmt/tree.go:119-134`; `datura/dmt/forest.go:162-203`.

### 69. Classification silently truncates classes and can panic on long names

**What is wrong or could be better.** Classification scratch has capacity for 16 classes and 48 bytes per name. Additional classes are ignored, and slicing the 48-byte name buffer to `len(className)` panics for a longer name.

**Reasoning.** The posterior can exclude valid competitors without indicating incompleteness, or untrusted/class-generated names can crash classification.

**Best solution.** Use reusable dynamically sized scratch with configured explicit limits. Return an error when a limit is exceeded and copy class names into owned storage.

**Source references.** `datura/dmt/cognitive_classification.go:14-45`; `datura/dmt/cognitive_classification.go:156-177`.

### 70. Classification results alias scratch and matching is bidirectional

**What is wrong or could be better.** The returned score slice and winner names point into reusable scratch. Reusing the scratch mutates an earlier result. `basinMatchesSequence` also treats a basin that extends the query as a match, so evidence for a more-specific path contributes to a shorter query.

**Reasoning.** Persisted or concurrent consumers can see results change after return, and classification semantics include future/suffix evidence that was not observed in the query.

**Best solution.** Return an owned immutable result and match only basins that are exact token-boundary prefixes of the observed sequence.

**Source references.** `datura/dmt/cognitive_classification.go:98-106`; `datura/dmt/cognitive_classification.go:237-251`.

### 71. Cognitive compare-and-swap retries can overwrite concurrent learning

**What is wrong or could be better.** Mutation builders read current counts and encode absolute replacement values before entering `commitLearnMutations`. If root CAS fails, the same stale mutations are applied to the newer root instead of recomputing increments.

**Reasoning.** Two concurrent learners updating the same path can both derive count `n+1`; one wins and the retry writes `n+1` again, losing an observation.

**Best solution.** Express learning operations as deltas and execute them in one serialized tree transaction that reads the current root, computes new values, and commits once.

**Source references.** `datura/dmt/cognitive_classification.go:289-359`; `datura/dmt/cognitive_engine.go:324-385`.

### 72. Cognitive commits publish memory before the WAL

**What is wrong or could be better.** `commitLearnMutations` swaps the new root first and calls `logLearnMutations` afterward. Logging failures are stored internally and not returned to the learning caller. Log indexes are derived from an atomic load outside the persistent tree mutex.

**Reasoning.** A crash can expose acknowledged in-memory learning that is absent after restart. Concurrent batches can allocate overlapping log indexes, and the caller receives success even when persistence failed.

**Best solution.** Route cognitive updates through the same serialized durable transaction as ordinary persistent inserts: compute under the transaction lock, append and fsync one WAL batch, publish the root, advance the index, and return any error.

**Source references.** `datura/dmt/cognitive_classification.go:355-412`; `datura/dmt/tree_persistence.go:37-81`.

### 73. WAL flush is treated as durability although it is only a userspace flush

**What is wrong or could be better.** Insert, batch insert, delete, and term logging flush `bufio.Writer` but do not sync the file before reporting success. Background sync errors are not part of the committing call, and `Close` discards the error from `walFile.Sync`.

**Reasoning.** A successful mutation can be lost on crash or power failure, and the caller cannot observe a final sync failure.

**Best solution.** Make each durability boundary explicit: WAL batch write, buffer flush, file fsync, then root publication. Return sync errors and allow a separately named relaxed mode only when loss is an accepted contract.

**Source references.** `datura/dmt/persist.go:159-300`; `datura/dmt/persist.go:307-340`.

### 74. Snapshot and WAL rotation do not establish one crash-safe checkpoint protocol

**What is wrong or could be better.** Snapshot creation and WAL rewriting are spread across separate operations, directory entries are not consistently synced, and replay recovery truncates tails without validating a monotonic term/index sequence across all records.

**Reasoning.** A crash between root snapshot publication, metadata update, and WAL replacement can leave ambiguous recovery state. Duplicate or regressing indexes can be replayed without being rejected.

**Best solution.** Write one complete snapshot to a temp file with term/index and checksum, fsync it, rename and sync the directory, then create and sync a new WAL that starts after that exact index. Replay must reject nonmonotonic term/index records.

**Source references.** `datura/dmt/persist.go:381 onward`; `datura/dmt/snapshot.go`; `datura/dmt/tree_persistence.go:130-153`.

### 75. Forest replication ignores write failures and does not replicate durability

**What is wrong or could be better.** `Forest.Insert` discards every `Tree.Insert` result. `synchronizeTrees` copies root, term, and index pointers into other trees without writing their persistent stores. `AddTree` accepts a tree without nil validation.

**Reasoning.** The forest can report no error while some replicas failed. Replicas appear synchronized in memory but restart from stale WALs, so restart changes which data exists.

**Best solution.** Use one authoritative durable commit log. Apply a committed entry to read replicas only after durability succeeds, track each replica’s applied index, and return commit errors from forest writes.

**Source references.** `datura/dmt/forest.go:113-141`; `datura/dmt/forest.go:249-262`.

### 76. The forest “fastest tree” policy can select an empty or stale replica

**What is wrong or could be better.** The selector starts with the first tree and compares average latency; a tree without meaningful samples can report zero. Root-pointer synchronization also means replicas share the same immutable data while retaining divergent persistence.

**Reasoning.** Read routing can prefer a replica based on missing metrics rather than health and applied index. The replication cost provides little data isolation while complicating recovery.

**Best solution.** Route reads only among replicas whose applied index equals the authoritative commit index and whose latency sample is valid. Remove replicas that only share the same in-process root without an independent operational purpose.

**Source references.** `datura/dmt/forest.go:144-203`.

### 77. ForestServer client construction misuses one `net.Pipe` endpoint

**What is wrong or could be better.** Each `Client` call creates a new RPC connection over the same `clientSide`, stores it in an unsynchronized map, and then returns the original local capability rather than a capability from the new connection. The constructor panics on invalid configuration, and `Done` is a no-op.

**Reasoning.** Concurrent clients race the map and multiplex unsupported connections over one endpoint, while the newly created connection is functionally unused. Lifecycle and errors are hidden behind panics or no-op methods.

**Best solution.** For in-process use, remove `net.Pipe` and expose one local Cap’n Proto capability with explicit reference ownership. Return constructor errors and synchronize the client registry only if remote transports are actually added.

**Source references.** `datura/dmt/server/server.go:42-105`; `datura/dmt/server/server.go:105-142`.

### 78. Exact RPC lookup performs analogical fallback

**What is wrong or could be better.** `Lookup` calls `IntelligentLookupPipeline`, which delegates to `GetAnalogousFallback`. Missing output positions remain zero-valued artifacts rather than carrying an explicit found bit.

**Reasoning.** A caller requesting an exact Morton key can receive a different key’s payload without knowing that fallback occurred, or cannot distinguish a miss from an empty artifact.

**Best solution.** Make exact lookup return `{found, value}` for each key. Expose analog search as a separate RPC that returns the matched key and similarity score.

**Source references.** `datura/dmt/server/server.go:142-234`; `datura/dmt/cognitive_reasoning.go:151-175`.

### 79. Analog and entropy queries scan or truncate data in ways that change semantics

**What is wrong or could be better.** `FindStructuralAnalog` scans the entire tree across namespaces. Entropy and next-token routines use fixed-size buffers and stop when capacity is reached in radix order, not probability order.

**Reasoning.** Analog latency grows with the complete database. Classification and ambiguity can ignore higher-probability branches that happen to appear later lexicographically.

**Best solution.** Maintain namespace-specific child indexes with top-k selection by probability and a token-prefix index for analog candidates. Queries must report truncation explicitly.

**Source references.** `datura/dmt/cognitive_reasoning.go:93-175`; `datura/dmt/cognitive_engine.go:80-139`.

### 80. Beam search allocates full paths and sorts every expansion

**What is wrong or could be better.** Each expansion copies a complete byte sequence, appends all candidates to a slice, and fully sorts it before truncating. Stochastic selection uses the process-global RNG.

**Reasoning.** Allocation grows with depth × branching, and results are not reproducible across tests or independent engines.

**Best solution.** Use an arena of parent-linked beam nodes and a bounded top-k heap. Inject a per-engine RNG with a deterministic seed.

**Source references.** `datura/dmt/cognitive_engine.go:196-282`.

### 81. Decay consolidation is expensive and its factor is not tied to elapsed time

**What is wrong or could be better.** Decay counts a namespace, scans it again to build mutations, and compares every key against every preserved sequence. The decay factor is derived from replayed observations divided by replayed plus namespace entries.

**Reasoning.** Work grows roughly with entries × preserved paths, and a small replay over a large namespace can nearly erase probabilities regardless of how much time elapsed.

**Best solution.** Store last-observed event time with each weight and apply a configured event-time decay during the same serialized mutation transaction, using a hash set of preserved keys.

**Source references.** `datura/dmt/cognitive_reasoning.go:187-275`; `datura/dmt/cognitive_reasoning.go:362-395`.

### 82. `Map.Marshal` converts serialization failure into a nil payload

**What is wrong or could be better.** The helper logs a Sonic marshal error and returns `nil` rather than returning the error. Callers commonly enqueue the result without knowing whether serialization failed.

**Reasoning.** Malformed or unsupported values turn into silent empty frames and are indistinguishable from intentional absence.

**Best solution.** Change the API to `Marshal() ([]byte, error)` and make every boundary decide whether to reject, retry, or count the failed frame.

**Source references.** `datura/map.go`.

## 6. Build reproducibility and verification gaps

### 83. The supplied snapshots are not self-contained build inputs

**What is wrong or could be better.** No Go module manifests, frontend package manifest, TypeScript configuration, or CI workflow is present in the supplied files. Generated Cap’n Proto Go is present, but the root datura snapshot also references artifact APIs that are not represented in its visible top-level source set.

**Reasoning.** A clean checkout cannot be reproduced from these snapshots, dependency versions cannot be audited, and source compatibility cannot be verified end to end.

**Best solution.** Commit complete module and frontend manifests, generated-source provenance, and a clean-checkout build workflow that vendors or locks every dependency.

**Source references.** `supplied snapshot directory structures`.

### 84. Integration tests intentionally bypass the production actor cascade

**What is wrong or could be better.** The `cutOwned` fixture mode returns before wiring analyzer, planner, and crypto actors. `Stack.Observe` then runs those stages synchronously with an explicit reset.

**Reasoning.** Tests validate the better-behaved fixture orchestration rather than the production scheduling model where the cut-boundary defect occurs.

**Best solution.** Run integration fixtures through the same cut coordinator and actor inboxes as production, using deterministic in-memory transports and a virtual clock.

**Source references.** `symm/stack/boot.go:220-405`; `symm/stack/boot.go:413-437`.

### 85. Concurrency contracts are not exercised with the race detector

**What is wrong or could be better.** The code relies heavily on shared pointers behind `sync.Map`, callbacks, atomics, and lock-free queues. The supplied test structure contains ordinary unit tests and benchmarks but no visible race-focused CI or ownership assertions.

**Reasoning.** Many defects described above are schedule-dependent and will not be found by deterministic happy-path tests.

**Best solution.** Make `go test -race ./...` a required CI job and add stress tests that concurrently publish, reset, serialize, close, reconnect, and process account events.

**Source references.** `symm, nomagique, and datura test files in the supplied snapshots`.

### 86. Persistence behavior lacks crash-point verification

**What is wrong or could be better.** Persistence tests exercise normal replay and snapshots, but the commit protocols need validation at every write, flush, sync, rename, and root-publication boundary.

**Reasoning.** Normal unit tests cannot prove that acknowledged state survives a process or power failure.

**Best solution.** Add a deterministic fault-injection filesystem and restart the store after every possible failure point, asserting that recovery returns either the last acknowledged transaction or a clear fatal error.

**Source references.** `datura/dmt/persist_test.go`; `datura/dmt/tree_persistence_test.go`; `datura/dmt/merkle_snapshot_test.go`.

### 87. Core algebraic and queue invariants need property tests

**What is wrong or could be better.** Many tests assert selected examples, while queue empty/zero behavior, lattice equivalence, quantity fitting, event reordering, and classification capacity are state-space problems.

**Reasoning.** Example tests can encode the current questionable behavior rather than expose counterexamples.

**Best solution.** Add property and fuzz tests for FIFO linearizability, no manufactured queue values, exact tick-lattice identity, maximum funded quantity, idempotent execution replay, and posterior mass preservation under arbitrary class counts.

**Source references.** `datura/structure/*_test.go`; `nomagique/algorithm/book/flow/*_test.go`; `symm/broker and tests fixtures`.


## Architectural end state

The findings converge on one concrete system shape:

1. A market ingress owner normalizes venue frames once and assigns source sequence and event time.
2. A cut coordinator creates a `CutID`, fans typed input to signals, and waits for one result or explicit skip from every interested signal.
3. The analyzer and planner consume an immutable completed cut and return immutable analysis and command values.
4. A broker owner processes commands, account events, reservations, positions, and fills serially. It persists idempotent intent transitions before exposing them as committed.
5. UI and audit are downstream consumers with explicit coalescing or durability policies; they cannot block authoritative market or account ingestion.
6. `nomagique` stages are one-owner streaming objects with mathematically consistent readiness, bounded memory, stable units, and allocation-free hot paths.
7. `datura` exposes separate concurrent queue, immutable snapshot, and durable transaction APIs. Persistent mutation always logs and syncs before root publication.

That architecture removes the need for most `sync.Map` usage, makes race behavior testable, gives every event one temporal identity, and lets performance work target actual bounded queues and numerical kernels rather than scheduler-dependent shared mutation.
