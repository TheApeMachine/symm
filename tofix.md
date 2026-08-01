Fix the following issues. The issues can be from different files or can overlap on same lines in one file.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @AGENTS.md around lines 50 - 59, Update the “Test Structure Mirrors Code Structure” guidance in AGENTS.md by correcting “Benchmarsk” to “Benchmarks” and changing “BDD style nesting” to “BDD-style nesting”; leave the surrounding rules unchanged.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @broker/price.go around lines 114 - 158, Update the BUY case in WithFriction to calculate the price from tick.Ask, while preserving tick.Bid for SELL so the method matches Mark’s side-specific pricing behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @broker/price.go around lines 203 - 272, Normalize ticker symbols consistently in the ticker cache: update the storage key used by Price.Update and the lookup key in Tick to use price.api.Name(symbol). Preserve the existing nil behavior when no normalized ticker is found, and leave fee-key handling unchanged.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @cmd/root.go around lines 117 - 136, Refactor the repeated signal construction and readiness logging around the correlation, cvd, depthflow, exhaust, hawkes, leadlag, liquidity, pumpdump, sentiment, and toxicity symbols into a local flat slice of named constructor functions, then iterate it to build each subscription and emit the corresponding errnie.Info log. Reuse the resulting name-to-subscription entries when constructing the analyzer subscription map so each signal is registered in one place and new signals do not require duplicate updates.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @cmd/root.go at line 121, Update depthflow.NewSignal to return a descriptive error when flow.NewSample fails instead of returning a nil signal, then handle that error at the call site in the root command before invoking depthflow.Subscribe. Ensure startup exits through the existing error path and never operates on a nil *depthflow.Signal.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @component-direct-paint.md around lines 19 - 24, Update the documentation to describe the implemented data-paint, data-paint-format, and data-paint-class attributes used by component.tsx instead of data-transform, data-set, or data-update. Explicitly identify data-set, data-target, and data-append as planned or unsupported attributes so their current inert status is clear, referencing the emitting components such as kernel-list.tsx and kernels.tsx.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @component-direct-paint.md around lines 1 - 28, Rewrite the transcript in component-direct-paint.md as neutral design documentation rather than a verbatim chat message: add a top-level heading, describe the Component paint registration contract and local rendering scope, explicitly specify the supported data attributes and their alignment with paint(updates) payload keys, and document the relevant flat/typed payload shapes and retained-data behavior. Remove second-person commentary and conversational history, and ensure the file ends with a trailing newline.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/collections/types.ts at line 2, Replace every remaining frontend reference to StrategyDecision with the exported Decision type, including imports and annotations across affected components. Ensure no StrategyDecision references remain and preserve the existing type usage semantics.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/charts/fluid.tsx around lines 299 - 303, Update the guard and nearby comment in the wave-painting function to describe the current wave-only behavior, removing the obsolete picture reference. Ensure an empty wave still clears the overlay via overlay.clearRect before returning; preserve the direct-paint flow for non-empty wave data.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/charts/hawkes.tsx around lines 5 - 16, Replace the no-op Component-based registration in HawkesChart with an actual canvas painter that receives update payloads and draws the Hawkes visualization, preserving the "hawkes" painter registration contract. Restore or implement the appropriate paintHawkes flow, and update the stale comment so it references the active painter rather than the removed behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/charts/hawkes.tsx around lines 11 - 12, Update the wrapper div in the chart component, identified by the ref and cn(...) className, to include the relative positioning utility alongside its existing classes. Keep the canvas absolute inset-0 styling unchanged so it positions relative to this wrapper, matching the pattern used by the Cortex surface component.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/charts/signal-heatmap.tsx around lines 6 - 21, The TerminalSignalHeatmap currently registers the generic Component painter even though its canvas has no data-paint elements, so it never renders. Restore or implement a canvas-specific paintTerminalSignalHeatmap and register it explicitly via registerPainter("measurements", paintTerminalSignalHeatmap); also update the nearby comment to match the restored painter.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/charts/signal-heatmap.tsx around lines 12 - 16, Update the heatmap component’s painter registration to remove select="rows" and consume the measurements array provided by registerPainter("measurements", ...). Add the canvas [data-paint] target and implement the painter callback to draw the measurement data, ensuring updates reach and render on the canvas.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/terminal/cortex-beam-shell.tsx around lines 14 - 17, Fix CortexBeamShell's migration by either extending Component's target scanning and updateTargets behavior to support the data-cortex targets, style.display changes, and indexed pooled Panel rows, or retain the explicit cortex-beams painter for this shell. Ensure the waiting notice is hidden, the content container is revealed, and the appropriate pooled beam rows become visible when the painter updates.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/terminal/cortex-panels-shell.tsx around lines 15 - 18, Update CortexPanelsShell so its registered "cortex-panels" painter targets the shell root recognized by Component, using paintCortexPanels; apply the equivalent connection in CortexBeamShell with paintCortexBeams. Ensure both painters receive matching data-paint targets and continue using the existing shell hooks and rendering structure.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/terminal/cortex-surface.tsx around lines 13 - 30, Update the cortex surface around the registered “cortex” painter and header reading-count span so the count has a data-paint target instead of hardcoded “0 readings”, allowing received data to update it. Restore an explicit canvas painter for the sensory prefix-tree canvas, using the existing canvas drawing behavior rather than relying on Component text/class updates. Ensure the registered painter targets the canvas and the count target within this subtree.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/terminal/decisions-surface.tsx around lines 10 - 13, Restore candidate-ladder rendering in DecisionsSurface: either retain and register the explicit candidate painter, or add synchronization that creates candidate rows from array payloads and hides the data-decision="waiting" panel. Remove the stale comment reference to paintDecisions* exports unless those exports are restored, and ensure candidate frames no longer remain permanently in the waiting state.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/terminal/decisions-surface.tsx around lines 25 - 42, Update the stat card in the decisions surface so the hardcoded value `0` becomes a painter target using the appropriate `data-paint` value, while retaining the title and subtitle targets. Change the grid declaration from four columns to match the single rendered card unless the omitted cards are restored, and remove the unnecessary `cn` wrapper around the static value className.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/terminal/kernel-list.tsx around lines 34 - 38, Remove the no-op ref callback from the row element in the kernel list component, including its null check, since it performs no side effects and only causes unnecessary ref attach/detach work.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/terminal/kernel-list.tsx around lines 22 - 33, Update the source-row interaction around the button’s onClick handler to restore the non-compact inspector-opening behavior described by the decisions surface, while preserving selectSource(source) for compact mode. If non-compact inspection is intentionally unavailable, instead render the row as non-interactive and remove button affordances such as focusability, pointer cursor, and hover styling.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/ui/component.tsx around lines 150 - 175, Update selectScopedUpdates so an array payload with no dataset.scope or dataset.filter returns undefined instead of updates[0]; preserve direct returns for non-array payloads and scoped matching behavior for arrays.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/ui/component.tsx at line 3, Remove the unused slots ref and the ComponentRenderProps.slots API, including its render-prop argument and related creation at the component render flow. Consolidate the Paint type by exporting the existing JSONSerializable-based definition from a single module, import it in both component.tsx and ws-stores.ts, and remove the cast around the paint callback.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/ui/component.tsx around lines 83 - 109, Restore the data-set/data-target binding flow used by scanTargets and updateTargets so arbitrary property paths such as style.backgroundColor and style.width are applied to matching elements. Preserve the existing textContent and class-name updates, and normalize or consistently support the barWidth/bar_width keys used by the terminal and dashboard consumers.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/ui/component.tsx around lines 177 - 200, Update applyPaintClass to split each rule’s value after the first colon on commas and toggle every resulting class individually, preserving the existing expected-value matching. Then align the paint-class declaration in the kernel-list call site and the dashboard kernels call site to use the same comma-separated multi-class format.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/ui/component.tsx around lines 237 - 270, Update the component’s painter registration flow around useLayoutEffect to accept a stable registration key instead of an inline register callback. Resolve that key through registerPainter inside the component, and make the effect depend on the stable key so DOM scanning and registration do not repeat on ordinary renders. Update all callers, including the chart and dashboard kernel consumers, to pass the key while preserving existing paint behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/components/ui/component.tsx around lines 111 - 122, Update formatValue to validate the parsed fractional-digit count before calling Number.toFixed, rejecting missing, non-numeric, or out-of-range values with a descriptive error; remove the redundant boolean-specific formatting branch so booleans use the existing String fallback. Ensure updateTargets handles formatValue errors per target so one malformed data-paint-format does not abort the remaining paint pass.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/providers/websocket.test.ts at line 60, Update the test around connect() to reset or capture MockWorker.instances before connecting, assert that exactly two workers are created, and select the first captured worker as the main worker instead of using at(-2) or at(-1). Preserve the existing test behavior while making worker selection explicit and order-independent in the assertion setup.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/providers/ws-stores.ts around lines 31 - 33, Update the comment describing attach to state that it dispatches frame entries to registered painters, removing the outdated reference to drawers and paintThesis. Do not change the attach implementation.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/providers/ws-stores.ts around lines 37 - 41, Update the DRAW_BIN handling in the worker message listener within attach to use the current focus symbol from appStore.state.focusSymbol instead of the undeclared focusSymbol reference; preserve the existing repaintTerminalFluidChart call and message filtering behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/providers/ws-stores.ts around lines 38 - 41, Update the DRAW_BIN handling in ws-stores.ts to decode event.data.buffer, apply the decoded data to the chart state used by repaintTerminalFluidChart, and then repaint with the updated state. Define focusSymbol by retrieving it from the existing application state before calling repaintTerminalFluidChart, rather than relying on an undefined variable.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/routes/index.tsx around lines 23 - 34, Replace the local kernels array in the route component with the shared DEFAULT_KERNELS import from #/collections/app. Use DEFAULT_KERNELS to derive the header count and pass it to KernelList, removing the duplicate literal while preserving the existing rendering behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @frontend/src/types/thesis.ts around lines 4 - 9, Update the doc comment above the Action type export to refer to Decision instead of the removed StrategyDecision name, preserving the rest of the comment’s meaning.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/book.go at line 47, Synchronize concurrent access to Book.status: update the writes in the OnCreateBook and OnChecksummed callbacks and the read in Status() using a mutex or atomic.Value. Ensure utils.NewWaiter’s readiness polling observes status changes safely without unsynchronized reads or writes.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/conn.go around lines 112 - 117, Remove the API.Name passthrough method and update its callers to access the normalizer directly through api.Normalizer().Name(symbol), preserving existing name-normalization behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/conn.go around lines 98 - 103, Update Status to handle nil api.public or api.private connections before calling Conn.Status, returning types.PENDING when either connection is missing; preserve types.READY only when both non-nil connections report READY. Use the existing NewAPI and Status symbols, without changing unrelated connection behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/conn.go around lines 137 - 141, Update TradeVolume to copy the input symbols slice before normalizing entries, then normalize and pass only the copy onward so the caller’s slice remains unchanged; use index as the loop variable name.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/conn.go at line 65, Remove the unused API.level3 field and its initialization, leaving Live.level3 as the sole owner of active level3 state. After updating the API definition and constructor, remove the sync import if no other references remain.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/conn.go around lines 119 - 123, Update API.Subscribe to route account keys “executions” and “add_order” through api.private instead of api.public, while preserving public routing for other keys. Add the corresponding decoders to entityMap before dispatch so both private subscription payload types are decoded correctly.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/conn_test.go around lines 78 - 103, Update TestTradeVolume to use the default mock metadata with BTC/USD instead of mockapi.NewConn configured for ETH/USD, and key the mock fee by the canonical pair XXBT/ZUSD. Call TradeVolume with BTC/USD, assert tradeVolumeInput contains XXBT/ZUSD, and update the result fee lookup and expected value to XXBT/ZUSD while preserving the existing assertions.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/live.go around lines 186 - 221, Rework the ping loop started by the recurring OnConnected handler to cancel any existing ping context before creating a new one, and replace context.Value state with an atomic.Int64 request counter plus an atomic.Pointer[time.Time] or mutex-guarded last-pong timestamp updated by the receive callback. Read the ping interval from the existing configuration, check pong freshness on every tick, report a descriptive stale-pong error, and propagate/report the error returned by live.Write instead of discarding it; ensure request IDs increment safely without type assertions or races.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/live.go around lines 329 - 355, Update Live.SubL3 to avoid blocking its caller by moving batch subscription and pacing into context-aware asynchronous work. Replace hardcoded group size, batch size, depth, and delay with the existing configuration values, and pace batches using a ticker tied to live.ctx rather than time.Sleep. Validate the connection returned by New before storing or using it; report a descriptive error and skip the child when it is nil, preserving cancellation through the connection context.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/live.go at line 106, Update the normalizer setup in the live connection initialization to handle and propagate the error returned by live.normalizer.Use(live.client.REST), matching the errnie.Error wrapping used by NewAPI in conn.go. Do not ignore the failure or substitute a fallback.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/live.go around lines 231 - 250, Update the Live construction flow around live.client.Connect() to set the initial status before connecting, capture its returned error, and report it diagnostically instead of discarding it. Synchronize live.status across the OnConnected, OnDisconnected, OnAuthenticated callbacks and Status() reads using a mutex or atomic value, preserving the existing status transitions.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/live.go around lines 357 - 387, Guard the live.level3 access in both Live.Books and Live.Book: when level3 is nil, return the existing empty map from Books and a nil *book.Book from Book before calling Range. Preserve the current iteration and lookup behavior when level3 is initialized.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/live.go around lines 43 - 50, The handler map entries in live.go should return values directly instead of using immediately invoked functions: simplify the "pong" entry by unmarshalling into a local map within its handler, and replace the "status", "heartbeat", and "subscribe" boolean closures with direct true returns while preserving the existing behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @kraken/websocket/live.go around lines 133 - 141, Update the receive callback around the entityMap lookup to retrieve the handler with an existence check before invoking it. When the channel is missing, return a descriptive error instead of calling a nil function; preserve the existing channel/method fallback and handler invocation for known channels.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/analyzer.go around lines 121 - 135, Update the error message in Analyzer.process to describe a failed signal-to-Thesis conversion rather than specifically mentioning CVD, while preserving the existing errnie.Error(errnie.Err(...)) pattern and err variable naming guideline.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/analyzer.go around lines 71 - 88, Update the analyzer constructor’s struct literal to assign the incoming binui channel to the analyzer.binui field, alongside the existing manifold.NewSolver(api, ui, binui, recorder) usage; do not leave the field nil or remove it unless all analyzer.binui usage is eliminated.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/analyzer.go around lines 177 - 192, Update Analyzer.Subscribe to protect subscription registry updates with the analyzer mutex, replacing the non-atomic LoadOrStore/Store sequence and ensuring the stored subscriber slice is not concurrently aliased with readers. Use the existing mutex and subscriber registry representation consistently, and keep returning the newly added subscription.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/analyzer.go around lines 166 - 175, Update the broadcast iteration in onSignal to process only the "thesis" subscription key before sending the thesis to its subscribers. Preserve the existing subscriber type check and Send behavior, and do not broadcast theses to subscriptions registered under other keys.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/analyzer.go around lines 90 - 119, Update Analyzer.run to validate analyzer.subscriptions before dereferencing any Subscription.Channel, including handling a nil map and all required signal keys. Build the channel set once from the populated map, return or report a descriptive error identifying each missing required signal, and only start processing after validation succeeds; preserve serialized analyzer.process execution unless the existing design explicitly supports concurrent updates.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/analyzer_test.go around lines 12 - 29, Strengthen TestAnalyzerOnSignal by replacing the hand-built types.NewThesis fixture with a multi-leg replay from the tests/market system, then verify both partial-thesis suppression and publication once the replay produces a ready thesis. Update the unexpected-channel-value assertion to use a clear Convey message with So(false, ShouldBeTrue), or assert the drained value directly.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/causal/solver.go around lines 141 - 142, Update the audit logging call in the causal solver flow to wrap the result of audit.Record with errnie.Err before passing it to errnie.Error. Use the existing stage context and a descriptive message so the logged error includes both context and an error code, and follow the guideline of naming the intermediate error variable err if one is introduced.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/cognition/solver.go at line 192, Update the audit error handling in the solver flow around audit.Record to wrap the returned error with errnie.Err using descriptive stage context and an error code, then pass that wrapped error to errnie.Error. Preserve the existing predictive audit operation and recorder usage.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/graph/solver.go around lines 183 - 193, Update the audit error handling in the graph solver’s Update flow to match the causal, cognition, resonance, and manifold solvers: log recorder failures without returning them, allowing propagation to continue. If an error must still be returned at this layer, wrap it with errnie.Err and include graph stage context, but preserve the established cross-solver behavior of not aborting on transient audit failures.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/manifold/solver.go around lines 177 - 191, Remove the redundant recorderOrNil passthrough function and pass solver.recorder directly to audit.Record in the solver recording block. Wrap the resulting audit error with errnie.Err so the returned error includes a descriptive message.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @logic/resonance/solver.go around lines 189 - 190, Update the audit-recording calls in Update, including the predictive stage and the additional occurrence near the later stage, to return or propagate the error produced by audit.Record through errnie.Error instead of invoking it standalone. Ensure Update reports the audit write failure rather than returning nil.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/correlation/signal.go around lines 80 - 94, Replace the non-atomic LoadOrStore/Store logic in Signal.Subscribe with a mutex-guarded subscription map that copies the channel’s slice before appending, preventing lost registrations and aliasing with onTicker readers. Apply the same change to the duplicated Subscribe implementations in the cvd and depthflow Signal types, preferably by extracting and reusing a shared helper in the types or signal package.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/correlation/signal.go around lines 117 - 126, Update the thesis fan-out in the signal handling path around the subscribers loaded under "thesis" so delivery cannot block on a full subscription buffer; use the established non-blocking overflow policy or a separate delivery worker. Before publishing, create an immutable thesis snapshot (or otherwise serialize access) so consumers receive stable data and cannot share the mutable object that Planner.Update resets.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/correlation/signal_test.go at line 64, Update the warmup call around consume to use an explicit named slice variable for discarded measurement results, then pass its address to consume; preserve the existing market.Warmup assertion and behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/cvd/signal.go around lines 125 - 134, The thesis broadcast logic is duplicated in onTicker and onTrade, causing the same blocking pattern in both paths. Extract it into a publishThesis method on Signal, then replace both inline subscriber-loading and Send loops with calls to publishThesis while preserving the existing missing-subscriber behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/cvd/signal_test.go around lines 16 - 44, Move the duplicated non-blocking drain logic from drainCVDTickers and drainCVDTrades, along with the equivalent helpers in signal/correlation/signal_test.go, into one generic helper in the tests package. Update both signal test packages to reuse that helper while preserving ticker/trade filtering and collected data behavior; keep the tests organized with the existing GoConvey structure.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/cvd/signal_test.go around lines 109 - 110, Strengthen the assertions in the pump test around the SIM1/USD entries so pump[types.MetricDrive] and absorption[types.MetricAbsorption] are each verified to be greater than their corresponding baseline values, rather than merely different. Preserve the existing metric keys and test structure while asserting the required increase direction.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/depthflow/signal_test.go at line 85, Update the warmup call in the test to make discarded rows explicit: either pass a named throwaway measurement slice to consume or adjust consume to accept nil and skip appending, then preserve the existing nil assertion for market.Warmup.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/depthflow/signal_test.go around lines 100 - 108, Strengthen the assertions in the depthflow test around the thin, loaded, and spoof score lookups so each focused “SIM1/USD” metric is also required to be strictly greater than zero, while preserving the existing comparisons against the baseline values.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/exhaust/signal_test.go around lines 16 - 44, Move the duplicated drain logic from drainExhaustTrades and drainExhaustBooks into generic shared helpers in the tests package, then update these helpers and the equivalent drainDepth* and drainHawkesTrades usages to call them with the appropriate trade or book type. Remove the local duplicate implementations while preserving each test’s existing collected data behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/exhaust/signal_test.go around lines 88 - 109, Update TestCalculate to either measure and compute the baseline urgency peak, then assert the rejection urgency exceeds it, or remove the unused baseline measurement and revise the Convey description to match the remaining assertion. Do not retain a full baseline run without using its urgency data.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/hawkes/cut_test.go around lines 18 - 23, The repeated Hawkes causal-state initialization and planner wiring should use one shared test helper. Extract the three Causal.Store calls and Planner/Signal construction into a package-level helper, then replace the duplicated setup in cut tests, newTestSignal, and measureHawkes with calls to that helper.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/hawkes/cut_test.go around lines 19 - 23, Extend the tests around Signal construction to call NewSignal instead of only manually assembling Signal and its Planner. Verify that NewSignal initializes the three expected Causal entries for the sample, process, and mutex, while preserving the existing behavior assertions.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/hawkes/market_test.go at line 1, Rename or move market_test.go to signal_test.go and rename TestMarketCalculate to TestCalculate so the test file and function mirror the Signal.Calculate implementation and required test naming convention.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/hawkes/market_test.go around lines 66 - 85, Restructure TestMarketCalculate into nested GoConvey blocks: keep the market fixture setup and measurement collection in an outer Convey, then place the event scan and foundEvents assertion in an inner Convey. Preserve the existing fixture, metric conditions, and assertion behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/hawkes/signal.go around lines 120 - 131, Extract the duplicated thesis subscriber fan-out from onTicker and onTrade into a shared publishThesis method on the signal type. Move the nil-check, "thesis" subscriber lookup, type assertion, and subscriber.Send calls into that method, then have both handlers invoke it while preserving the existing behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/hawkes/signal.go around lines 88 - 107, The Subscribe method has a racy lazy initialization and non-atomic subscriber update. Apply the same mutex-guarded map approach used by signal/exhaust: initialize subscribers safely before concurrent use, lock the read-modify-write when adding to a channel, and ensure onTicker/onTrade access is synchronized with Subscribe.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/hawkes/signal_test.go around lines 90 - 103, Extend the newTestSignal helper to initialize the thesis causal entries and any subscriptions required by this test, then replace the inline thesis and Signal construction in the test with a call to newTestSignal. Preserve the test-specific setup while reusing the shared fixture.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/leadlag/signal.go around lines 122 - 133, Extract the thesis subscriber fan-out block into a named publishThesis method on the signal type, then replace the inline logic in the current signal flow with a call to that method. Preserve the existing nil guard, “thesis” lookup, subscription type handling, and Send behavior inside publishThesis so the subscriber contract is centralized.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/leadlag/signal.go around lines 74 - 93, Update Signal.Subscribe to use the same mutex-guarded subscriber map pattern as signal/exhaust/signal.go. Protect initialization and append/update operations on signal.subscribers with the existing synchronization mechanism, ensuring concurrent Subscribe calls cannot race with onTicker or overwrite registrations.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/leadlag/signal_test.go around lines 74 - 92, Harden the assertions in the lead/lag test around measureLeadlag and the pump/baseline peak measurements: ensure the requested MetricSync value is actually asserted and require a positive lower bound so all-zero coordination evidence cannot pass. Preserve the existing MetricStrength comparison while adding the corresponding non-zero MetricSync validation for each symbol, or remove MetricSync from metrics if it is not intended to be tested.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/leadlag/signal_test.go around lines 28 - 52, The pre-warmup ticker drain in measureLeadlag duplicates the consume closure logic. Reuse consume with a throwaway result target to process drainLeadlag(tickerSub), removing the copied loop while preserving the thesis tick increment and signal.Calculate calls.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/liquidity/signal.go around lines 127 - 138, Extract the thesis subscriber fan-out currently in the signal notification flow into a package method named publishThesis. Move the nil-map guard, "thesis" lookup, type assertion, iteration, and subscriber.Send calls into that method, then replace the duplicated block with a call to publishThesis.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/liquidity/signal.go around lines 79 - 98, The Subscribe method unsafely initializes and updates subscribers concurrently, allowing registrations to race or be lost. Apply the mutex-guarded subscriber map pattern used by signal/exhaust/signal.go: protect initialization and the complete LoadOrStore/append update in Subscribe, and ensure onTicker accesses signal.subscribers under the same mutex.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/pumpdump/signal_test.go around lines 17 - 60, Move the repeated non-blocking channel-drain logic from drainPumpTickers, drainPumpTrades, drainPumpBooks, drainToxicityTrades, drainToxicityBooks, and drainSentiment into one generic helper in the tests package. Update each signal test to call that shared helper while preserving type filtering and accumulated frame data behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/pumpdump/signal_test.go around lines 113 - 122, Update TestCalculate by nesting the ignition and trend So assertions in separate inner Convey blocks within the existing “Pumpdump raises ignition and trend on fast directional tapes” block. Keep each assertion’s comparison and test data unchanged so the two behaviors report independently.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/pumpdump/signal_test.go at line 74, Remove the no-op bookflow.NewBook reference and delete the now-unused bookflow import from the test.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/sentiment/signal.go around lines 76 - 95, Make subscriber registration atomic by replacing the duplicated Subscribe implementations with one shared implementation that protects the per-channel slice using a mutex (or an equivalent mutex-protected slice), avoiding concurrent read-modify-write and backing-array mutations. Remove lazy signal.subscribers initialization from Subscribe and retain the single initialization in NewSignal; update the sentiment, toxicity, cvd, correlation, and depthflow signals to use the shared implementation.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/sentiment/signal.go around lines 124 - 135, Decouple the subscriber loop in onTicker from blocking Subscription.Send calls by routing deliveries through a bounded worker or queue. Define and apply an explicit overflow policy for full queues, such as dropping the update, while keeping ticker ingestion non-blocking and preserving delivery to available subscribers.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/sentiment/signal_test.go around lines 88 - 92, Strengthen the surge validation in the test loop over the pump symbols so a missing or zero-valued MetricSurgeScore cannot pass; require a strictly positive surge or compare the pump peak against the baseline-state peak, following the established pattern in the pumpdump signal test. Keep the existing isolated MetricDivergentScore assertion unchanged.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @signal/toxicity/signal_test.go around lines 11 - 12, Remove the duplicate marketkraken import and retain the unaliased kraken import in the test file. Update the TradeVolumeResult reference to use the kraken identifier, preserving the existing kraken.TradeData reference.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/allocator.go around lines 39 - 48, Update Allocator.Allocate after allocator.balance.Cash() succeeds to validate that budget is non-nil and positive before any budget.Mul or cost.Cmp calls. Return a descriptive error for missing or non-positive budgets, while preserving the existing Cash error handling and normal allocation flow for valid budgets.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/allocator.go around lines 82 - 87, Remove the stale err != nil condition from the cost guard following allocator.price.WithFriction in the allocation flow. Keep the existing cost == nil and budget comparison checks unchanged, since err is already handled before this point and WithFriction returns only a value.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/allocator.go around lines 94 - 98, Update the allocation flow around WithFriction to assign ReferencePrice from allocator.price.Mark(pair.Symbol, broker.BUY), which provides the per-unit price, rather than the notional ask value. Check the Mark result for nil and reject the decision before calling Copy() when no market price is available.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/arbiter.go around lines 122 - 124, Rename the remaining single-character receiver on Arbiter methods, including Arbitrate, to the descriptive arbiter name used by getIncumbents. Update all references within those methods and keep behavior unchanged.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/evaluator.go around lines 324 - 328, Update the forecast impact calculation near forecast.FrictionReady in the evaluator flow to derive the impact coefficient from measured market depth or realized slippage instead of using the hardcoded 0.1 multiplier. Reuse the existing observed-data/statistical inputs and preserve the ExpectedSpread-to-ExpectedImpact calculation and readiness assignment.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/evaluator.go around lines 304 - 322, Update the error branch in Evaluator.stampFriction so a failed evaluator.price.Fee lookup does not set forecast.FrictionReady to true; leave it false while preserving the existing zero-value assignments and error reporting, ensuring Eligible excludes forecasts with unknown friction.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/evaluator.go around lines 418 - 424, Update EvaluateOpportunities to validate or rehydrate all evaluator dependencies before calling getOccupiedSymbols or filtering forecasts. Ensure desk is non-nil before invoking Positions(), and initialize nil Thesis pointer maps used by getOccupiedSymbols, getCognition, inspectGraph, and isExiting, including values loaded through trader.JournalStore.Load. Preserve normal behavior for already-initialized dependencies and allow zero-value or directly constructed Thesis inputs to evaluate without panics.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/planner.go around lines 152 - 156, Update the readiness checks in the planner flow to reuse the initial readiness snapshot instead of recomputing thesis.Readiness() after allocation; if recomputation is intentional, document that intent. In the Decisions check near the later readiness validation, replace the explicit “== false” comparison with boolean negation while preserving the existing control flow.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/planner.go around lines 72 - 92, Make AttachAnalyzer idempotent by avoiding replacement of an existing "thesis" subscription and ensuring planner.run() starts only once, including when NewPlanner already attached an analyzer. Reuse the existing subscription/run-state guard across both NewPlanner and AttachAnalyzer so repeated or late analyzer attachment cannot create duplicate consumers.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @strategy/planner_test.go around lines 11 - 24, Add a test case alongside TestPlannerUpdate that marks every readiness stage as complete, registers a subscriber via planner.Subscribe, and invokes Update. Assert the subscriber receives the expected []types.Decision payload, then verify the thesis has been reset to its post-Reset state, following the readiness setup pattern from TestThesisReadiness.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @trader/crypto.go around lines 93 - 111, Update decisionsReady to remove reflection-based slice handling and the unused Crypto receiver, accepting only the concrete []types.Decision payload produced by the planner. Change the decision validation flow to report an unexpected payload with a descriptive error instead of silently returning false, and remove the now-unused reflect import while preserving the non-empty decision requirement.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @trader/crypto.go around lines 76 - 78, Update the boolean guard around crypto.decisionsReady(decisions) to use the idiomatic negated form, replacing the explicit == false comparison while preserving the existing continue behavior.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @types/decision.go at line 38, Update Forecasts.Eligible to guard ExpectedAdverseSelection and any other nil decimal fields before dereferencing them, returning an ineligible result when required values are absent. Preserve the existing eligibility checks for populated decimals; do not rely on Decision.AdverseSelection validation tags, since ManageContinuation invokes Eligible before creating the Decision.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @types/forecasts.go around lines 30 - 37, The validation tags on ExpectedReturn, ExpectedFees, ExpectedSpread, ExpectedImpact, and ExpectedAdverseSelection must require non-nil decimal pointers because Eligible dereferences them. Add required to each existing tag while preserving the current finite and nonnegative constraints.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @types/forecasts.go around lines 68 - 72, Update Eligible() to validate that ExpectedReturn, ExpectedFees, ExpectedSpread, ExpectedImpact, and ExpectedAdverseSelection are all non-nil before constructing the values slice or calling Float64()/Sign(). Apply the same validation to ExecutableReturn(), or otherwise make incomplete friction data fail explicitly before its Sub() chain dereferences nil values.

- Verify each finding against current code. Fix only still-valid issues, skip the rest with a brief reason, keep changes minimal, and validate.

In @types/forecasts_test.go around lines 41 - 48, Update the ExecutableReturn assertion in the “It should subtract every friction component” test to convert the returned *decimal.Decimal to a numeric value before passing it to ShouldAlmostEqual, while preserving the expected 0.03 result.