# Market → Sensorium ontology experiment

This implements an executable baseline/A/B comparison, not a claim that either
candidate is a useful market model. Trading policy and its six scalar manifold
inputs retain their contract. Candidates run through `tools/manifoldexperiment`
on retained public market frames, outside the live trading agent. The baseline
uses the current `Dataset`; A/B share the same Sensorium engine with explicit
source and boundary semantics. The engine's verified population-count and field
layout fixes also apply to the ordinary path.

## 1. Existing ontology

The baseline independently normalizes log price and log quantity into X/Y,
uses whole-side rank for Z and phase, derives frequency from price, and assigns
quantity/Hawkes-derived energy, mass, and heat. This repeats inputs across
physical variables and overlays symbol-local coordinate systems. The comparison
measures its resulting sheet; it does not presume the sheet's cause.

## 2. Original research recovered

The seven Python files were read under the adjacent
`sensorium-manifold/backup/sensorium` tree: `tokenizer/compressor.py`,
`tokenizer/universal.py`, and the five requested `experiments` files.
Their transferable principles are structural location, separate content identity,
semantic compression into mass, unit initial oscillator energy, emergent modes,
and free counterfactual probes. The original implementation actually seeds zero
heat despite a contrary comment. Toy-image experiments are not evidence for
market prediction. No byte-specific frequency formula was transplanted.

## 3. Composed implementation

- `correlation.Relations` retains pair readings; `Signal.Relations` exposes that
  same owner, without O(N²) Grid columns.
- `SymbolGeometry` owns supported graph distances, MDS and alignment.
- `ArrivalCadence` and `PriceCarrier` compose the existing adaptive residual
  estimator. `BookProjection` owns identities and the market projection.
- `MarketTape` consumes exact capture identities and the live `websocket.Book`,
  correlation Ticker and trade Hawkes implementations.
- `Comparison`, `ProjectionRun` and `Diagnostic` own the research experiment,
  isolated physics domains and observations. They are not a second live book.
- Sensorium `Source` separates external coupling from oscillator energy;
  `Boundary` configures spatial topology in the existing engine.

The full current files are listed in the verification manifest. This is an
allocation-heavy offline experiment, not a 640-symbol per-L3-message deployment.

## 4. Exact candidate particle mapping

| Field | Meaning |
|---|---|
| ContentID | Monotonic collision-free handle for `(symbol, venue order ID)`, retained while resident |
| Bytes | Collision-free symbol handle, not a byte-derived frequency |
| TokenID | Handle for `(symbol, side, depth level, FIFO)`; different levels never collide |
| Seq | Zero-based FIFO within each level; reset at each boundary |
| X/Y | Aligned relationship coordinates × explicit geometry length unit + domain midpoint |
| Signed depth | Bid `-(level+0.5)`, ask `+(level+0.5)`; zero is the execution interface |
| Z | Signed depth × explicit depth unit + domain midpoint |
| Mass | Remaining base quantity × configured mass unit; additive, never squashed |
| Initial Energy/Amp | 1/1; subsequently owned by physics |
| Phase | Integral of prior L3 cadence through arrival time; evolved phase is retained |
| Omega | Current measured L3 angular cadence × seconds per solver unit |
| Heat | `mass × Cv × T0`, with engine `Cv=1`, tested `T0=0` |
| Velocity | Zero on authoritative clamped restatement; free probes retain integrated motion |
| Clamped/Dark | Observed orders true/false; dark probes remain free even if marked clamped |
| Source.Weight | A: `q_i/Q_s`; B: `C_s q_i/Q_s`, only for known arrival phase |
| Source.Drive | Opposite-side aggressive trade excess intensity × `2π` × seconds per solver unit |

Snapshot/backfill arrival phase is **unobserved**, not reconstructed from later
data. Such matter retains mass and unit oscillator initialization but has zero
external source weight. Diagnostics retain the defined flag. Unknown initial
phase still affects the engine's unweighted Kuramoto readout, so that scalar is
not automatically authority about observed arrival coherence.

Birth position, current projection seed and evolved position are distinct in
telemetry. Authoritative updates relocate clamped orders when their level or
symbol geometry changes; they do not overwrite evolved oscillator energy or
phase. Mass changes preserve resident temperature, hence change heat as an
explicit material boundary update.

## 5. Symbol price carrier

`C_s = exp(log(p_s) - prior adaptive log-price baseline)`: price relative to its
causal geometric baseline. First observation gives unit carrier with zero
maturity. Raw price and carrier maturity are recorded. A quote denomination
change multiplies numerator and baseline together. No historical min/max window
or directional trading label is used.

Particle amplitude remains `sqrt(Eosc)`. The spectral accumulator receives that
amplitude times `Source.Weight`; source-weighted anchors use the same coupling.
A symbol contributes `C_s Σ_i (q_i/Q_s) Amp_i exp(i phase_i)` over observed-phase
orders. Missing arrival authority attenuates this contribution explicitly; it
is not redistributed into invented coherent orders. Mode amplitude remains the
engine's evolved result. Equal fragments are compressed before this operation.

## 6. Correlation → geometry

The API retains signed/absolute Hayashi correlation, overlapping-pair support,
Fisher defined state, p-value, standard error and the older path endpoint.
**Independence-adjusted effective support and calibrated authority are unavailable
and represented as null**, not mislabeled raw counts. The geometry experiment
uses the existing independent-return Fisher approximation; its confidence
interpretation has not been established for dependent asynchronous returns.

Given configured family-wise alpha and pair-age budget, the lower absolute
correlation bound is `tanh(max(0, atanh(abs(rho)) - z*SE))`, with Bonferroni
`z=Φ⁻¹(1-alpha/[n(n-1)])`. Positive bounds supply edges with distance
`sqrt(2*(1-bound))`. Missing edges stay missing; connected paths use shortest
path distance. Disconnected universes wait with an explicit reason. Classical
MDS retains at most two positive dimensions and reports negative eigenmass and
retained variance. Zero-dimensional perfect dependence is allowed.

Sorted labels, eigenvector sign fixing and orthogonal Procrustes alignment remove
iteration order and equivalent rotation/reflection ambiguity. Alignment does
not suppress real distance changes. Invalidated geometry cannot be projected.
Sign remains explicit relationship evidence; negative dependence is close by
absolute strength. It does **not** impose temporal phase opposition. Lead/lag is
unused and supplies no invented direction, significant or otherwise.

## 7. Cadence and Hawkes

A new L3 ADD advances the adaptive positive inter-arrival-gap estimator. Phase
integrates the *previous* natural frequency over the new gap; then omega becomes
`2π / adaptive mean gap`. Simultaneous timestamps share phase and do not invent
a zero-gap frequency; this is cadence at the observed timestamp resolution,
not an independently fitted multiplicity-aware intensity. Regressed historical
backfill remains unobserved. Trade Hawkes excess-arrival intensity supplies an
external radian/time drive, never natural omega, heat or initial energy.

The command explicitly configures seconds per solver unit. Physics still
advances one existing solver step per accepted common book update; this is an
experiment time convention, not calibrated equivalence between solver time and
elapsed exchange time. Out-of-domain omega fails a variant visibly rather than
being silently mapped into a boundary frequency bin.

## 8. Compression and representation tests

Only `FragmentOf` declares pieces of the same resident observation. Such pieces
must agree on symbol, side, level, FIFO, price and arrival time; quantity sums.
Different venue orders and different roles are never merged merely because a
hash, location or timestamp agrees. Four controlled cases run actual Metal
physics: one Q=6 order; three equivalent Q=2 fragments; three FIFO roles; three
levels. The first two have identical physical outputs. The latter cases retain
three particles and conserved total mass; levels occupy different spatial cells.
Carrier normalization also passes denomination and fragmentation tests.

The captured book's FIFO is the SDK timestamp queue. Equal timestamps with no
proved tie priority are rejected by the research adapter, rather than assigning
venue priority from lexicographic IDs. No such ambiguity was skipped in the
successful reported prefixes.

## 9. Topology audit and transport fixes

Candidate X/Y/Z are closed. CIC indexing, spatial field sampling/gradients,
minimum-image coupling, collision neighborhoods, grid stencils, spatial mode
projection and probe advection use this domain setting. Gas ghost cells reflect
normal momentum; probes reflect at walls. Opposite depth faces are not neighbors.
Observed particle positions are restored after gather, with velocity cleared;
gas/wave fields continue to evolve. Dark probes are tested to move.

The frequency-lattice DFT remains periodic: it is not book-depth topology.
The optional periodic gravitational Poisson path is inactive in this pipeline
(`gravity_enabled=0`); enabling it in a closed experiment requires a separate
Poisson-boundary implementation. Boundary code is specialized per Metal engine,
not process-global. The metallib was regenerated and cold zero-temperature and
opposite-face tests exercise the real kernels.

The added crash log was reproduced as an absent-level SDK dereference: enforcing
depth per order could evict a level before a later deletion in the same frame.
Depth is now enforced after the full symbol update. Missing-level deletion
invalidates the book and requests a fresh subscription/snapshot. Recovery sends
unsubscribe then subscribe with explicit depth and observes write/ack errors.
This follows Kraken's [L3 checksum procedure](https://docs.kraken.com/exchange/guides/websockets/l3-checksum-v2).
The SDK's `SubL3` ignored its depth argument, so subscription uses its explicit
private subscribe API. A dead dashboard writer now detaches its failed socket.

## 10. Captured comparison measurements

See [measurements](manifold-projection-verification/measurements.md) and the JSON
reports for exact counts, mass, entropy, midpoint occupancy, every mode, carrier
contribution, phase coherence, divergence, pressure gradient and wave scales.
All variants consume the same checksum-validated causal books after geometry
admission. Waiting is common to all variants; a failed variant stays failed.
Repeated runs and denomination/order/universe sensitivities are retained.
The failed SOL-universe attempt is retained too: SOL had no authoritative book
in this public prefix, so zero physics steps is **not** replay success.

## 11. Numerical and rendering findings

The audit exposed a real active-population bug: scatter counted and reordered
buffer *capacity*, then deposited stale tail rows. A grow/shrink regression
measured 153 mass where six should remain before the correction. All scatter
stages and PIC gather now receive active counts. Deposited mass then agrees with
resident mass to float32 accumulation precision. This is a buffer-contract fix,
not a new gas equation.

PackFields publishes component peaks. The renderer now multiplies by their
reciprocals (zero stays empty), and texture dimensions/sampling preserve Metal's
Z-fastest layout. Density, momentum, energy and complex wave share that contract.
Actual WebGPU pixel readback verifies that `1e-8` and unit waves render identically
on a non-cubic grid. This restores visibility; it does not make a tiny noisy
field scientifically meaningful.

## 12. Surprises

Strong supported BTC/ETH dependence in the selected prefix is negative. Its
geometry is one-dimensional, so candidate matter forms a Y-midpoint sheet for a
legitimate structural reason. The baseline's sheet lies on a different axis.
Removing all sheets would therefore be an incorrect success criterion.

## 13. Failures retained

GPU replay is not bitwise identical, and the longer baseline divergence is
material: the maximum repeat scalar difference reaches 494.58854. An independent
reordered comparison differs from the original baseline by up to 428.22686,
while A/B differ by approximately 0.000268/0.000124. These aggregate maxima
are not normalized uncertainty estimates. Deterministic input/projection tests
are separate from Metal reduction order and evolved-field stability. The existing eight phase-offset spectral heads are
averaged without undoing their offsets. Symmetric head phasors can cancel, so
the very small spatial wave may be cancellation-scale residue despite nonzero
per-head coherence. This equation was not changed to improve the picture.
The initial JSON exporter also broke decimal checksum lexemes; export now
embeds original payload bytes and has an exact-decimal/compression regression.

## 14. Unresolved interpretation and operating limits

- Raw base-quantity mass is additive within a symbol. Summing BTC and ETH base
  units does not establish a common economically or physically calibrated mass.
- Effective independent support, signed physical coupling and lead/lag transport
  are not established. The current Fisher graph is a falsifiable approximation.
- Adaptive geometry can still move authoritative sources when evidence really
  changes. Source motion, solver time and energy exchange need calibration.
- Unit energy per genuinely distinct FIFO order means order-count sensitivity
  remains in oscillator/thermal dynamics even when carrier weight is normalized.
  That is different from duplicating an explicitly equivalent representation.
- Existing eight-anchor selection limits spatial mode representation. A normalized
  source is not proof that the full multi-scale physics is fragmentation-neutral.
- Exact GPU equality is not claimed. No live venue reconnect/endurance run, agent
  learning validation, forecast quality or profitability claim follows from this
  offline experiment. The user database was read-only and was not discarded.
- Full MDS is cubic and full-book diagnostics allocate. The measured 640-symbol
  cost rules out putting this implementation on every production L3 event.

## 15. Recommended next experiment

Keep trading on its existing projection while testing the eight-head phase
reference and source/gas energy budget in controlled fields. Compare per-head
and combined mode phasors before changing equations. Then repeat on longer,
checksum-complete multi-symbol captures with a defensible common mass unit and
independence-aware pair uncertainty. Assess predictive content of the unchanged
six scalar outputs only after those physical questions are resolved.

## Verification

[Exact commands and literal outputs](manifold-projection-verification/verification.md)
include repository tests, targeted race checks, benchmarks, frontend checks,
Metal regeneration and actual WebGPU pixel readback. The structured research
reports and their command files are adjacent; no hidden promotion flag exists.
