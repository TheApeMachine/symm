# SYMM Metric Semantic Map

**Status:** normative information-architecture map  
**Baseline:** `993c147ea9a3eb371c5b51d3210088bd8aec708a`  
**Scope:** the 432 current projected `(source, metric)` identities plus their declared cross-signal semantics.

This document answers one question for every production metric:

> **Why does this fact exist, what may it legitimately affect, what may affect its interpretation/estimability, and what must never be inferred from it?**

It is deliberately not a wiring wish-list. A metric does not become useful by connecting it to something. Its edge must have a declared semantic type.

## 1. Non-negotiable model

```text
Signals measure.
Metrics retain units, definedness, causal support and provenance.
Relations state how facts interact.
Advisors compose descriptive context.
Perspectives describe.
Opportunity hypothesizes.
Valuation estimates economic consequences.
MCTS chooses among estimable actions, including keeping cash.
PositionRisk owns executable position economics.
Hindsight judges the chain afterward.
```

No stage is repaired by inventing a generic confidence scalar or binary evidence gate.

## 2. Typed relationship vocabulary

| Edge | Meaning | What it is NOT |
|---|---|---|
| `DERIVES_FROM` | Mathematical dependency required to calculate a metric. | A causal market claim. |
| `CONTEXTUALIZES` | Another fact changes how the metric should be interpreted. | A multiplier or vote. |
| `CONDITIONS_EFFECT` | Another state changes the expected consequence of a measured quantity; e.g. flow under shallow vs deep liquidity. | Reliability/confidence weight. |
| `DECOMPOSES` | Joint facts separate mechanisms that one metric conflates; e.g. arrival frequency vs trade size. | Evidence averaging. |
| `INFORMS_UNCERTAINTY` | Variance, SE, p-value, fit diagnostics, SNR or support describe uncertainty/model adequacy. | Directional evidence. |
| `SUPPORTS_ESTIMABILITY` | Defines whether a requested estimate is mathematically available. | `if maturity > 0.7` gating. |
| `TEMPORALLY_RELATES` | Describes precedence/alignment/excitation timing. | Causality or leadership. |
| `HISTORICAL_CONTEXT` | Baseline, divergence, recurrence or novelty relative to causal history. | Forecast of recurrence outcome. |
| `COMPOSES_IN` | Declared input to an Advisor/Category/causal model/Opportunity/Risk/Hindsight calculation. | Permission to trade. |
| `REDUNDANT_WITH` | Compatibility projection duplicates a canonical source. | Reason to keep both forever. |
| `FORBIDDEN_USE` | Explicit semantic prohibition. | Suggestion. |

## 3. What 'weight' means

There are three distinct concepts that MUST NOT be collapsed:

1. **Epistemic support / estimability.** `Maturity`, effective sample count, overlap support, estimator variance, Fisher SE, p-values, model-fit diagnostics, feed age and definedness answer whether/how precisely a calculation is known. They modify uncertainty or make a requested valuation undefined. They do not change the sign of the measurement and are not generic coefficients.
2. **Conditional market effect.** The same aggressive flow can have a different expected price consequence in a shallow book than in a deep book. Liquidity therefore conditions a flow→response relationship. This belongs in a declared relation/causal model, not `flow * liquidityConfidence`.
3. **Semantic corroboration.** Distinct measurements may jointly support a typed hypothesis, but dependence between them must be accounted for. Publication count, peer count, repeated aliases and duplicated legacy signals are never extra votes.

A calculation that needs a fact which is not yet estimable remains **undefined**. The system should not fabricate zero, wait an arbitrary boot duration, or turn missing knowledge into a confidence gate.

## 4. Canonical signal roles

- **`correlation` (34 metrics):** Asynchronous contemporaneous price-return dependence, support, return energy and historical movement of that dependence.
- **`cvd` (40 metrics):** Executed aggressive flow: event counts, economic size, side imbalance, rates and contemporaneous midpoint response.
- **`depthflow` (35 metrics):** Displayed full-book mutation: depth, imbalance, additions/removals, turnover, resolution gap and their dynamics.
- **`derivatives` (34 metrics):** Derivative/reference geometry: OI, basis/return gaps and liquidation economics.
- **`exhaustion` (30 metrics):** Legacy compatibility bundle duplicating liquidity/depthflow/CVD-style support facts; normative destination is removal after migration. **LEGACY/REDUNDANT: migrate and remove.**
- **`hawkes` (53 metrics):** Marked event-arrival dynamics: intensity, excitation, branching, fit diagnostics and innovations.
- **`leadlag` (29 metrics):** Temporal alignment geometry between explicitly related price paths.
- **`liquidity` (11 currently projected metrics):** Displayed executable touch capacity, price spread and side asymmetry. The restored specification additionally defines causal historical depth/spread state, divergence dynamics, recurrence, joint Maturity/SNR, and optional full-book morphology; most of that richer surface is not implemented yet.
- **`morphology` (7 metrics):** Dimensionless arrangement/shape of the L3 book and structural change.
- **`pumpdump` (37 metrics):** Legacy-named volume-clock activity signal: completed economic throughput, spread and midpoint response; not a pump/dump classifier. **Legacy package name: this is volume-clock activity, not a pump/dump detector.**
- **`sentiment` (63 metrics):** Legacy-named cross-sectional price-state signal: explicit cohort breadth, dispersion, return distribution and focal extremeness; not social sentiment. **Legacy package name: this is cross-sectional price state, not social sentiment.**
- **`toxicity` (59 metrics):** Touch-level liquidity disposition: fill, unexplained withdrawal, replenishment and retreat; not intent or maliciousness.

## 5. Normative cross-signal relation graph

| Left | Right | Type | Meaning |
|---|---|---|---|
| `cvd` | `liquidity` | `CONDITIONS_EFFECT` | Displayed capacity/spread conditions the economic price response to a given executed-flow state; compare aggression to touch capacity rather than treating flow magnitude alone as meaning. |
| `cvd` | `toxicity` | `CONTEXTUALIZES` | Touch fill/replenishment/withdrawal/retreat explains how displayed liquidity was disposed while aggressive flow executed. |
| `cvd` | `depthflow` | `CONTEXTUALIZES` | Compare actual executed flow with displayed-book mutation, turnover, and touch/full-book imbalance disagreement. |
| `cvd` | `hawkes` | `DECOMPOSES` | Arrival intensity × mean trade notional separates event frequency from economic event size; many-small and few-large activity are distinct. |
| `cvd` | `derivatives` | `CONTEXTUALIZES` | OI, basis and liquidation state contextualize aggressive flow without assigning leverage causality. |
| `cvd` | `correlation` | `CONTEXTUALIZES` | Local flow distinguishes a correlated price move with local participation from one with little local execution imbalance. |
| `cvd` | `sentiment` | `CONTEXTUALIZES` | Local signed flow contextualizes member/cohort return distribution and breadth. |
| `depthflow` | `liquidity` | `CONTEXTUALIZES` | Touch capacity/spread and full-book mutation answer different liquidity questions; use both to distinguish local touch changes from redistribution. |
| `depthflow` | `toxicity` | `CONTEXTUALIZES` | Aggregate removals/turnover plus touch-level trade attribution distinguish execution, withdrawal, retreat, replenishment and redistribution. |
| `depthflow` | `hawkes` | `CONTEXTUALIZES` | Book mutation under ordinary arrivals differs from mutation during clustered arrival intensity. |
| `depthflow` | `morphology` | `CONTEXTUALIZES` | Morphology describes arrangement; depthflow describes mutation. Shape change plus turnover/flow describes how structure is moving. |
| `depthflow` | `correlation` | `CONTEXTUALIZES` | Relationship changes under unusual book turnover/imbalance differ from the same price dependence under ordinary microstructure. |
| `depthflow` | `leadlag` | `CONTEXTUALIZES` | Lag changes can be interpreted alongside turnover, resolution gap, withdrawal/replenishment; never convert precedence into causality. |
| `liquidity` | `hawkes` | `CONDITIONS_EFFECT` | Expected notional arrival rate relative to displayed touch capacity describes potential executable-touch turnover rate. |
| `liquidity` | `leadlag` | `CONTEXTUALIZES` | Spread/depth state contextualizes lag seconds, lag gain and lag stability; liquidity may alter repricing speed. |
| `liquidity` | `correlation` | `CONTEXTUALIZES` | Correlation changes under shallow/wide vs deep/tight liquidity are different market configurations. |
| `liquidity` | `toxicity` | `CONDITIONS_EFFECT` | A given fill/withdrawal fraction has different economic size under large vs tiny absolute displayed depth. |
| `liquidity` | `sentiment` | `CONTEXTUALIZES` | The same breadth/dispersion under deep vs shallow liquidity is different context. |
| `hawkes` | `toxicity` | `CONTEXTUALIZES` | Arrival clustering combined with replenishment/withdrawal/retreat distinguishes event density from liquidity disposition. |
| `hawkes` | `leadlag` | `TEMPORALLY_RELATES` | Compare event-process excitation changes with price-path lag changes; neither establishes the other. |
| `hawkes` | `correlation` | `CONTEXTUALIZES` | Price-path coupling and event-process excitation are separate dependence structures. |
| `hawkes` | `derivatives` | `TEMPORALLY_RELATES` | Liquidation events require distinct marks if modeled; compare liquidation clustering with OI/liquidation state. |
| `correlation` | `leadlag` | `TEMPORALLY_RELATES` | Correlation measures dependence at a defined alignment; lead-lag measures how dependence changes with alignment. |
| `correlation` | `sentiment` | `CONTEXTUALIZES` | Pair/cohort dependence complements breadth, return dispersion and median movement. |
| `correlation` | `derivatives` | `CONTEXTUALIZES` | For explicit related markets, price dependence complements basis, return gap, OI and liquidation state. |
| `leadlag` | `sentiment` | `CONTEXTUALIZES` | Cross-sectional state may motivate explicit pair analysis but must not silently pick the leader/reference. |
| `leadlag` | `derivatives` | `TEMPORALLY_RELATES` | Spot/perpetual/index lag complements basis/OI/liquidation geometry; it is not price discovery by itself. |
| `sentiment` | `derivatives` | `CONTEXTUALIZES` | Cohort breadth/dispersion can be compared with OI, basis and liquidation distributions in explicit derivative cohorts. |
| `pumpdump` | `cvd` | `CONTEXTUALIZES` | Volume-clock economic throughput complements signed executed flow; activity and direction are separate. |
| `pumpdump` | `liquidity` | `CONDITIONS_EFFECT` | Activity/return under shallow vs deep capacity and spread state has different mechanical meaning. |
| `pumpdump` | `depthflow` | `CONTEXTUALIZES` | Volume-clock activity with turnover/book mutation distinguishes tape throughput from displayed-book change. |
| `pumpdump` | `toxicity` | `CONTEXTUALIZES` | Activity and return combined with replenishment/withdrawal/retreat describe how the touch responds. |
| `pumpdump` | `hawkes` | `DECOMPOSES` | Completed economic throughput and event-arrival intensity separate event count dynamics from trade size. |
| `pumpdump` | `sentiment` | `CONTEXTUALIZES` | Local activity/return is interpreted against explicit cohort breadth and dispersion. |
| `pumpdump` | `correlation` | `CONTEXTUALIZES` | Local activity state contextualizes changing pair/cohort price dependence. |
| `morphology` | `liquidity` | `CONTEXTUALIZES` | Book arrangement and absolute executable capacity answer different questions; shape is not capacity. |
| `morphology` | `depthflow` | `CONTEXTUALIZES` | Book arrangement and its mutation/turnover jointly describe structural evolution. |
| `morphology` | `toxicity` | `CONTEXTUALIZES` | Touch disposition can explain whether an observed shape change came with fills, replenishment, withdrawal or retreat. |
| `morphology` | `historical` | `HISTORICAL_CONTEXT` | Compare dimensionless morphology state/change to its own causal historical context in an Advisor; raw shape is not a regime label. |
| `exhaustion` | `liquidity` | `REDUNDANT_WITH` | Legacy compatibility source duplicates canonical liquidity facts; migrate consumers to liquidity. |
| `exhaustion` | `depthflow` | `REDUNDANT_WITH` | Legacy compatibility source duplicates canonical depthflow facts; migrate consumers to depthflow. |
| `exhaustion` | `cvd` | `REDUNDANT_WITH` | Legacy compatibility source duplicates canonical executed-flow facts; do not grow new exhaust dependencies. |

### Important consequence

A large percentage of the current 'dead' metrics are not supposed to point directly at MCTS or an Advisor. Some are denominator/support facts, some are model diagnostics, some condition other relationships, and some should disappear because a canonical source already exists. **The target is not 432 green lineage dots. The target is 432 explained identities, followed by fewer production metrics.**

## 6. Current named semantic consumption

The current `CategorySchemas` provides named fine-grained use for a small subset. That is recorded here as **current behavior**, not automatically endorsed. Several mappings need review because the metric specification explicitly disallows the semantic conclusion when used alone.

| Metric | Current Category use | Review |
|---|---|---|
| `correlation/cohort_correlation_dispersion` | DivergentStress | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `correlation/cohort_signed_correlation` | SystemicHerd | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `correlation/correlation_divergence` | DivergentStress | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `correlation/correlation_zscore` | StochasticNoise | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `correlation/relative_return_energy_divergence` | DecoupledAlpha | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `correlation/relative_return_energy_zscore` | DecoupledAlpha | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `correlation/signed_correlation` | SystemicHerd | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `cvd/flow_aligned_midpoint_return` | StochasticBalance | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `cvd/gross_notional_rate_divergence` | VolumeStarvation | REVIEW: divergence sign/transform must be explicit before mapping to VolumeStarvation. |
| `cvd/gross_notional_rate_zscore` | HiddenAbsorption | REVIEW: high flow-rate unusualness alone does not establish HiddenAbsorption. |
| `cvd/midpoint_response_per_net_notional` | HiddenAbsorption | REVIEW: response efficiency can contextualize absorption but does not itself label HiddenAbsorption. |
| `cvd/midpoint_return_rate_zscore` | StochasticBalance | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `cvd/signed_net_fraction_divergence` | AggressiveDrive | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `cvd/signed_net_fraction_zscore` | AggressiveDrive, VerticalIgnition | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `depthflow/book_imbalance_divergence` | LoadedImbalance | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `depthflow/book_imbalance_zscore` | LoadedImbalance, VerticalIgnition | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `depthflow/net_book_change_rate` | BookThinning | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `depthflow/resolution_gap_zscore` | BookThinning, VerticalIgnition | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `depthflow/touch_imbalance` | SpoofTrap | REVIEW: one touch imbalance cannot semantically justify SpoofTrap. |
| `depthflow/turnover_zscore` | DenseNeutrality | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `derivatives/basis_rate` | DerivativesDecoupling | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `derivatives/basis_zscore` | DerivativesDecoupling | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `derivatives/liquidation_notional_rate` | AdverseLeverageBuildup | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `derivatives/liquidation_share` | LongDeleveraging | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `derivatives/liquidation_signed_fraction` | ShortSqueeze | REVIEW: side imbalance alone does not establish ShortSqueeze. |
| `derivatives/net_liquidation_notional` | LongDeleveraging | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `derivatives/open_interest_growth_rate` | LeveragedIgnition | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `derivatives/open_interest_growth_zscore` | LeveragedIgnition | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `exhaustion/book_imbalance_zscore` | MechanicalCollapse | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `exhaustion/depth_divergence_velocity:ask` | ActiveReversal | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `exhaustion/relative_spread` | FragileExpansion | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `exhaustion/spread_zscore` | ThermalExhaustion | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `exhaustion/total_depth_zscore` | MechanicalCollapse | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `hawkes/arrival_rate` | Inertial | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `hawkes/branching_spectral_radius` | Turbulent, VerticalIgnition | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `hawkes/excitation_intensity:buy` | Frenzy | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `hawkes/excitation_intensity:sell` | Frenzy | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `leadlag/best_lag_correlation_zscore` | AnchorStall | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `leadlag/contemporaneous_correlation` | SynchronizedDrift | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `leadlag/correlation_gain_zscore` | InefficientLag | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `leadlag/lag_fraction` | DecoupledMove | REVIEW: spec explicitly says lag_fraction is search-domain provenance, not universal relationship strength. |
| `leadlag/lag_zscore` | DecoupledMove | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `liquidity/relative_spread` | ExtremeScarcity | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `liquidity/touch_notional_imbalance` | ExtremeScarcity, RobustLiquidity | REVIEW: same raw metric currently supports both ExtremeScarcity and RobustLiquidity; polarity/transform is absent. |
| `liquidity/two_sided_touch_notional` | RobustLiquidity | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `pumpdump/midpoint_return_zscore` | OrganicTrend, FadedExhaustion | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `pumpdump/notional_rate_zscore` | VerticalIgnition | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `pumpdump/relative_spread` | CoiledCompression | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `pumpdump/spread_zscore` | CoiledCompression | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `pumpdump/trade_interval_seconds` | OrganicTrend | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `pumpdump/volume_bar_quantity` | VerticalIgnition, CoiledCompression | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `pumpdump/volume_rate` | VerticalIgnition | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `sentiment/advance_fraction` | SystemicSlump | REVIEW: positive advance fraction mapped to SystemicSlump requires explicit polarity transform. |
| `sentiment/breadth_zscore` | SystemicSlump | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `sentiment/directional_agreement` | RiskOnSurge | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `sentiment/directional_consensus` | RiskOnSurge | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `sentiment/median_absolute_return_zscore` | DivergentMove | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `sentiment/return_dispersion_zscore` | DivergentMove | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `toxicity/fill_fraction_zscore:ask` | LiquidityVacuum | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `toxicity/fill_fraction_zscore:bid` | LiquidityVacuum | No immediate contradiction identified; still requires declared polarity/transform and dependence semantics. |
| `toxicity/withdrawal_fraction_zscore:ask` | ToxicBluff | REVIEW: withdrawal unusualness can contribute to a bluff hypothesis only jointly; not proof of ToxicBluff. |
| `toxicity/withdrawal_fraction_zscore:bid` | ToxicBluff | REVIEW: withdrawal unusualness can contribute to a bluff hypothesis only jointly; not proof of ToxicBluff. |

Current Advisor bindings are also explicit and descriptive:

- `cvd/signed_net_fraction_zscore` → advisor:historical
- `depthflow/book_imbalance` → advisor:liquidity
- `depthflow/book_imbalance_zscore` → advisor:historical
- `hawkes/excitation_fraction:buy` → advisor:historical
- `liquidity/relative_spread` → advisor:liquidity
- `liquidity/touch_notional_imbalance` → advisor:liquidity

A generic measurement/kernel subscriber is **not** semantic use. It only means a component can see the stream. The map requires a named question/relationship before a metric counts as intentionally consumed.

## 7. Readiness / definedness rules

- Direct observables and stateless deterministic transforms are usable as soon as their source observations are valid.
- Historical baselines/divergences/z-scores/velocities are undefined until their own causal estimator prerequisites exist.
- Pair dependence/lag statistics are undefined without valid shared support; support counts and search breadth inform inference, never market direction.
- Hawkes fitted quantities are undefined when the model/parameter/compensator required for that quantity is not usable.
- Cross-sectional facts require an explicit cohort and valid common-horizon members; excluded/stale members are provenance, not zeros.
- Toxicity/disposition facts require a valid comparable book/trade bracket; ambiguity invalidates attribution.
- Morphology shape facts require a valid comparable two-sided L3 book; `morphology_change` additionally requires a prior comparable shape.
- Economic planning should run only when the **particular consequence it wants to estimate** is defined with its required context/uncertainty. This is native estimability, not a single global readiness bit.

## 8. Canonical-source / deletion decisions already implied by the specs

### `exhaustion`

The current Exhaust spec explicitly describes the package as a compatibility bundle over facts whose canonical homes are Liquidity, Depthflow and CVD. Therefore all 30 current `exhaustion/*` projections are **migration targets, not new composition inputs**. Existing consumers should move to canonical metrics and the duplicate projections should then be removed.

### `pumpdump`

Keep the volume-clock mathematics; remove the semantic debt of the package name over time. Its metrics describe activity, throughput, spread and midpoint response. They do not measure 'pump' or 'dump'.

### `sentiment`

Keep the explicit-cohort cross-sectional mathematics; rename eventually. It measures price-state breadth/dispersion/extremeness, not human/news sentiment.

### `liquidity`

The restored `signal/liquidity/README.md` is now the canonical Liquidity contract.

It defines substantially more than the current 11-metric touch implementation:

- **core touch state:** executable bid/ask prices, quantities, notionals, midpoint, spread, relative spread, two-sided capacity and touch asymmetry;
- **causal historical state:** side-specific depth baselines, ratios, log divergences, noise scales and z-scores;
- **spread history:** relative-spread baseline, ratio, divergence and z-score;
- **event-time dynamics:** bid/ask depth-divergence velocity, spread-divergence velocity and slope SNR;
- **historical recurrence:** nearest-path distance, percentile and prior-match provenance;
- **joint quality:** effective-support Maturity and covariance-aware multivariate SNR.

The current `signal/liquidity/ticker.go` still projects only the 11 core touch metrics. Therefore the missing historical/dynamic/recurrence metrics are **spec-declared implementation gaps**, not dead metrics and not license to fabricate ad-hoc consumers.

The restored spec also gives precise cross-signal relationships:

```text
aggressive buy notional / displayed ask capacity
aggressive sell notional / displayed bid capacity

arrival intensity / opposite-side displayed capacity

touch state + toxicity disposition
touch geometry + deeper-book mutation
price/correlation state + liquidity divergence/SNR/novelty
volume-rate state + liquidity depth/spread/shape dynamics
```

These are typed conditioning/context relationships, not votes or confidence multipliers.

#### Morphology ownership conflict

The restored historical Liquidity spec also defines optional full-book morphology under Liquidity. The current architecture, however, now has a dedicated `signal/morphology` producer.

Those two surfaces MUST NOT both become canonical producers for equivalent book-shape facts.

Current mapping decision for this document:

- `signal/liquidity` remains canonical for **touch capacity, spread, and their own historical state**;
- `signal/morphology` remains the current canonical producer for **full-book normalized morphology**;
- Liquidity §11/§12/§16 is marked **SPEC_CONFLICT_WITH_SIGNAL_MORPHOLOGY** until the repository spec is reconciled;
- missing historical Liquidity metrics remain owned by Liquidity and are listed in `metric_spec_gaps.csv`.

This is an explicit architecture discrepancy, not something lineage should hide.


## 9. Complete current metric catalog

The tables below contain all **432 current projected `(source, metric)` identities** at the baseline commit. `Current named use` records only current Category/Advisor use established from the static declarations reviewed here; blank does not mean 'delete immediately'—it means the metric needs a typed role before further semantic wiring.

### 9.1 `correlation` — 34 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `last_price` | Current price sample retained to form the return path. | `RELATIONSHIP_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `observation_count` | Support/provenance: number of retained price observations, not relationship strength. | `RELATIONSHIP_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `signed_correlation` | Direction and strength of contemporaneous return dependence for the evaluated relation/cohort. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | Category:SystemicHerd | `KEEP_NAMED_USE` |
| `absolute_correlation` | Magnitude of contemporaneous return dependence independent of sign. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `cohort_signed_correlation` | Focal symbol's aggregate signed dependence to its explicit peer cohort. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | Category:SystemicHerd | `KEEP_NAMED_USE` |
| `cohort_absolute_correlation` | Focal symbol's aggregate dependence magnitude to its explicit peer cohort. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `covariance` | Unnormalised shared return variation; diagnostic/derivation input, not cross-symbol score. | `RELATIONSHIP_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `return_energy:reference` | Integrated return variation for the reference path; denominator/support geometry for dependence, not alpha. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `return_energy:measured` | Integrated return variation for the measured path; denominator/support geometry for dependence, not alpha. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `return_energy_rate:reference` | Return quadratic-variation rate over shared time; measures movement energy per event time, not direction. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `return_energy_rate:measured` | Return quadratic-variation rate over shared time; measures movement energy per event time, not direction. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `peer_return_energy_rate` | Return quadratic-variation rate over shared time; measures movement energy per event time, not direction. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `focal_return_energy_rate` | Return quadratic-variation rate over shared time; measures movement energy per event time, not direction. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `supported_return_count:measured` | Number of supported return increments on that path; inference support only. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `supported_return_count:reference` | Number of supported return increments on that path; inference support only. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `shared_time` | Elapsed time jointly represented by the compared paths; support provenance. | `RELATIONSHIP_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `overlap_density` | Density of supported return overlap in shared time; sampling/support context. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `overlap_pair_count` | Number of matched return pairs used by the dependence estimate; support only. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `effective_sample_count` | Effective independent support for correlation inference; informs uncertainty/definedness. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `correlation_p_value` | Inferential p-value for the measured correlation under its stated assumptions; reliability evidence, not trading probability. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `correlation_standard_error_fisher` | Fisher-space standard error of correlation; uncertainty of the estimate. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `cohort_peer_count` | Number of explicit peers represented; cohort provenance, not evidence weight. | `RELATIONSHIP_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `cohort_correlation_dispersion` | Heterogeneity of focal correlations across peers; distinguishes one common relationship from mixed peer coupling. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | Category:DivergentStress | `KEEP_NAMED_USE` |
| `cohort_effective_peer_count` | Effective peer support after dependence/weighting; uncertainty/support provenance. | `RELATIONSHIP_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `relative_return_energy` | Focal-vs-peer return-energy ratio/contrast describing relative movement magnitude. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `relative_cohort_return_energy` | Focal-vs-cohort return-energy contrast. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `correlation_baseline` | Causal historical reference level of correlation for this relation. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `correlation_divergence` | Current correlation departure from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | Category:DivergentStress | `KEEP_NAMED_USE` |
| `correlation_zscore` | Standardized unusualness of current correlation relative to its own history. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:StochasticNoise | `KEEP_NAMED_USE` |
| `correlation_velocity` | Rate at which the current correlation state is changing. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `relative_return_energy_baseline` | Causal historical reference for relative return energy. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `relative_return_energy_divergence` | Departure of focal-vs-peer movement energy from its historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | Category:DecoupledAlpha | `KEEP_NAMED_USE` |
| `relative_return_energy_zscore` | Standardized unusualness of relative movement energy. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:DecoupledAlpha | `KEEP_NAMED_USE` |
| `relative_return_energy_velocity` | Rate of change of relative movement energy. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |

### 9.2 `cvd` — 40 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `trade_count` | Executed trade-event count over the retained interval. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `trade_count:buy` | Executed trade-event count on the buy aggressor side. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `trade_count:sell` | Executed trade-event count on the sell aggressor side. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `signed_count_fraction` | Aggressor-side imbalance by event count; separates event-frequency imbalance from economic-size imbalance. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `executed_quantity:buy` | Executed base quantity on the buy aggressor side. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `executed_quantity:sell` | Executed base quantity on the sell aggressor side. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `gross_executed_quantity` | Total executed base quantity independent of side. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `net_executed_quantity` | Signed executed base-quantity imbalance. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `aggressive_notional:buy` | Executed quote-currency notional initiated on the buy side. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `aggressive_notional:sell` | Executed quote-currency notional initiated on the sell side. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `gross_notional` | Total executed economic notional independent of side. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `net_notional` | Signed aggressive executed notional. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `signed_net_fraction` | Scale-free signed share of gross aggressive notional; primary directional executed-flow state. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `mean_trade_notional` | Mean economic size per execution; separates many-small from few-large trade activity. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `trade_rate` | Execution event frequency per second. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `gross_notional_rate` | Total aggressive economic throughput per second. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `net_notional_rate` | Signed aggressive notional accumulation per second. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `buy_notional_rate` | Aggressive buy notional throughput per second. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `sell_notional_rate` | Aggressive sell notional throughput per second. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `cumulative_volume_delta` | Epoch-anchored cumulative signed base quantity; use interval differences, never compare arbitrary epochs. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `cumulative_notional_delta` | Epoch-anchored cumulative signed notional; use interval differences, never compare arbitrary epochs. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `cvd_epoch_from` | Origin/provenance of cumulative CVD state. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `signed_net_fraction_baseline` | Causal historical reference for `signed_net_fraction`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `signed_net_fraction_divergence` | Current departure of the underlying `signed_net_fraction_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | Category:AggressiveDrive | `KEEP_NAMED_USE` |
| `signed_net_fraction_zscore` | Standardized historical unusualness of the underlying `signed_net_fraction_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:AggressiveDrive,VerticalIgnition; advisor:historical | `KEEP_NAMED_USE` |
| `gross_notional_rate_baseline` | Causal historical reference for `gross_notional_rate`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `gross_notional_rate_ratio` | Scale-free ratio form of the underlying `gross_notional_rate_ratio` quantity for contextual/historical comparison. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `gross_notional_rate_divergence` | Current departure of the underlying `gross_notional_rate_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | Category:VolumeStarvation | `KEEP_REVIEW_CURRENT_USE` |
| `gross_notional_rate_zscore` | Standardized historical unusualness of the underlying `gross_notional_rate_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:HiddenAbsorption | `KEEP_REVIEW_CURRENT_USE` |
| `net_notional_rate_velocity` | Event-time rate of change of the underlying `net_notional_rate_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `gross_notional_rate_velocity` | Event-time rate of change of the underlying `gross_notional_rate_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `response_midpoint:from` | Causal quote midpoint associated with the start of the flow observation. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `response_midpoint:at` | Causal quote midpoint at the current end of the flow observation. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `midpoint_log_return` | Midpoint price response over the flow interval, separated from execution price. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `midpoint_return_rate` | Event-time-normalized midpoint response. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `flow_aligned_midpoint_return` | Midpoint return expressed in the direction of net aggressive flow; alignment fact, not confirmation or causation. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:StochasticBalance | `KEEP_NAMED_USE` |
| `midpoint_response_per_net_notional` | Price response per unit signed net notional; measures flow/response efficiency, undefined at zero net notional. | `FLOW_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:HiddenAbsorption | `KEEP_REVIEW_CURRENT_USE` |
| `midpoint_return_rate_baseline` | Causal historical reference for `midpoint_return_rate`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `midpoint_return_rate_divergence` | Current departure of the underlying `midpoint_return_rate_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `midpoint_return_rate_zscore` | Standardized historical unusualness of the underlying `midpoint_return_rate_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:StochasticBalance | `KEEP_NAMED_USE` |

### 9.3 `depthflow` — 35 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `book_notional:bid` | Displayed L3 notional depth on the bid side. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `book_notional:ask` | Displayed L3 notional depth on the ask side. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `book_notional` | Displayed L3 notional depth across both sides. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `book_imbalance` | Full-book bid-vs-ask displayed-notional asymmetry. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | advisor:liquidity | `KEEP_NAMED_USE` |
| `touch_imbalance` | Best-touch bid-vs-ask asymmetry; compare with full-book imbalance rather than treat as standalone spoof evidence. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:SpoofTrap | `KEEP_REVIEW_CURRENT_USE` |
| `imbalance_resolution_gap` | Difference between touch-level and full-book imbalance; measures disagreement across depth resolution. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `imbalance_resolution_distance` | Magnitude of touch/full-book imbalance disagreement independent of sign. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `net_displayed_flow:bid` | Net displayed-book notional change on the bid side. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `net_displayed_flow:ask` | Net displayed-book notional change on the ask side. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `added_notional:bid` | Displayed notional added on the bid side. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `removed_notional:bid` | Displayed notional removed on the bid side. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `added_notional:ask` | Displayed notional added on the ask side. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `removed_notional:ask` | Displayed notional removed on the ask side. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `net_displayed_flow_rate:bid` | Net displayed-book notional change on the bid side per second. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `net_displayed_flow_rate:ask` | Net displayed-book notional change on the ask side per second. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `added_notional_rate:bid` | Displayed notional added on the bid side per second. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `removed_notional_rate:bid` | Displayed notional removed on the bid side per second. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `added_notional_rate:ask` | Displayed notional added on the ask side per second. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `removed_notional_rate:ask` | Displayed notional removed on the ask side per second. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `book_turnover_rate` | Gross displayed-book mutation rate relative to book scale; activity of book replacement, not directional flow. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `net_book_change_rate` | Net total displayed-depth change rate relative to book scale. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:BookThinning | `KEEP_NAMED_USE` |
| `signed_net_displayed_flow_rate` | Signed bid-vs-ask net displayed-flow rate. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `flow_activity_imbalance` | Which side accounts for more gross displayed-book mutation activity. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `book_imbalance_baseline` | Causal historical reference for `book_imbalance`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `book_imbalance_divergence` | Current departure of the underlying `book_imbalance_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | Category:LoadedImbalance | `KEEP_NAMED_USE` |
| `book_imbalance_zscore` | Standardized historical unusualness of the underlying `book_imbalance_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:LoadedImbalance,VerticalIgnition; advisor:historical | `KEEP_NAMED_USE` |
| `book_imbalance_velocity` | Event-time rate of change of the underlying `book_imbalance_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `resolution_gap_baseline` | Causal historical reference for `resolution_gap`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `resolution_gap_divergence` | Current departure of the underlying `resolution_gap_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `resolution_gap_zscore` | Standardized historical unusualness of the underlying `resolution_gap_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:BookThinning,VerticalIgnition | `KEEP_NAMED_USE` |
| `resolution_gap_velocity` | Event-time rate of change of the underlying `resolution_gap_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `turnover_baseline` | Causal historical reference for `turnover`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `turnover_ratio` | Scale-free ratio form of the underlying `turnover_ratio` quantity for contextual/historical comparison. | `BOOK_MUTATION_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `turnover_zscore` | Standardized historical unusualness of the underlying `turnover_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:DenseNeutrality | `KEEP_NAMED_USE` |
| `turnover_velocity` | Event-time rate of change of the underlying `turnover_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |

### 9.4 `derivatives` — 34 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `derivative_price` | Current derivative trade/last price. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `reference_price` | Current explicitly supplied reference/index price. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `open_interest` | Current open interest in contract units; absolute value needs contract-unit context. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `basis` | Dimensionless derivative/reference price basis. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `log_basis` | Log ratio of derivative to reference price; symmetric basis geometry. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `derivative_index_log_basis` | Derivative vs index log basis; valid only with genuinely distinct legs. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `index_spot_log_basis` | Index vs spot log basis; valid only with genuinely distinct legs. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `derivative_spot_log_basis` | Derivative vs spot log basis; valid only with genuinely distinct legs. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `basis_closure_error` | Three-leg basis consistency error; diagnostic of derivative/index/spot geometry, not alpha. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `open_interest_change` | Absolute change in open interest; contract-scale quantity. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `open_interest_log_change` | Scale-free log OI change. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `open_interest_growth_rate` | Log OI growth per second. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:LeveragedIgnition | `KEEP_NAMED_USE` |
| `open_interest_growth_velocity` | Event-time rate of change of the underlying `open_interest_growth_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `open_interest_growth_baseline` | Causal historical reference for `open_interest_growth`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `open_interest_growth_zscore` | Standardized historical unusualness of the underlying `open_interest_growth_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:LeveragedIgnition | `KEEP_NAMED_USE` |
| `basis_change` | Change in basis. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `basis_rate` | Basis movement per second. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:DerivativesDecoupling | `KEEP_NAMED_USE` |
| `basis_velocity` | Event-time rate of change of the underlying `basis_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `basis_baseline` | Causal historical reference for `basis`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `basis_zscore` | Standardized historical unusualness of the underlying `basis_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:DerivativesDecoupling | `KEEP_NAMED_USE` |
| `derivative_log_return` | Derivative price log return over the observation interval. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `reference_log_return` | Reference price log return over the observation interval. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `return_gap` | Derivative return minus reference return; decoupling fact, not explanation. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `return_gap_velocity` | Event-time rate of change of the underlying `return_gap_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `return_gap_zscore` | Standardized historical unusualness of the underlying `return_gap_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `UNMAPPED_REVIEW` |
| `liquidation_notional:buy` | Liquidation notional on the buy side. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `liquidation_notional:sell` | Liquidation notional on the sell side. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `gross_liquidation_notional` | Total liquidation notional independent of side. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `net_liquidation_notional` | Signed liquidation notional. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:LongDeleveraging | `KEEP_NAMED_USE` |
| `liquidation_signed_fraction` | Side imbalance of liquidation notional. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:ShortSqueeze | `KEEP_REVIEW_CURRENT_USE` |
| `liquidation_notional_rate` | Liquidation economic throughput per second. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:AdverseLeverageBuildup | `KEEP_NAMED_USE` |
| `gross_derivative_trade_notional` | Total derivative trade notional used as denominator/context for liquidation share. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `liquidation_share` | Fraction of derivative traded notional represented by liquidation events. | `DERIVATIVE_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:LongDeleveraging | `KEEP_NAMED_USE` |
| `liquidation_share_velocity` | Event-time rate of change of the underlying `liquidation_share_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |

### 9.5 `exhaustion` — 30 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `displayed_depth_notional:bid` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `displayed_depth_notional:ask` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `displayed_depth_notional` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `spread` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `relative_spread` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:FragileExpansion | `MIGRATE_AND_REMOVE` |
| `book_imbalance` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `previous_book_imbalance` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `book_imbalance_change` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `midpoint_log_return` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `book_imbalance_baseline` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `book_imbalance_zscore` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:MechanicalCollapse | `MIGRATE_AND_REMOVE` |
| `book_imbalance_velocity` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `depth_baseline:bid` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `depth_divergence:bid` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `depth_divergence_velocity:bid` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `depth_ratio:bid` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `depth_zscore:bid` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `MIGRATE_AND_REMOVE` |
| `depth_baseline:ask` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `depth_divergence:ask` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `depth_divergence_velocity:ask` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | Category:ActiveReversal | `MIGRATE_AND_REMOVE` |
| `depth_ratio:ask` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `depth_zscore:ask` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `MIGRATE_AND_REMOVE` |
| `total_depth_baseline` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `total_depth_ratio` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `total_depth_zscore` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:MechanicalCollapse | `MIGRATE_AND_REMOVE` |
| `relative_spread_baseline` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `spread_divergence` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `spread_divergence_velocity` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `MIGRATE_AND_REMOVE` |
| `spread_ratio` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `MIGRATE_AND_REMOVE` |
| `spread_zscore` | Legacy compatibility projection of a liquidity/depth/imbalance/spread fact. Its normative purpose is migration to the canonical source, not new reasoning. | `DEPRECATE_REDUNDANT` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:ThermalExhaustion | `MIGRATE_AND_REMOVE` |

### 9.6 `hawkes` — 53 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `event_count` | Observed marked trade-arrival count. | `ARRIVAL_MODEL_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `event_count:buy` | Observed marked trade-arrival count for buy events. | `ARRIVAL_MODEL_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `event_count:sell` | Observed marked trade-arrival count for sell events. | `ARRIVAL_MODEL_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `event_fraction:buy` | Fraction of observed arrivals carrying the buy mark. | `ARRIVAL_MODEL_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `event_fraction:sell` | Fraction of observed arrivals carrying the sell mark. | `ARRIVAL_MODEL_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `arrival_rate` | Empirical observed arrival frequency across marks. | `ARRIVAL_MODEL_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:Inertial | `KEEP_NAMED_USE` |
| `arrival_rate:buy` | Empirical observed arrival frequency for buy events. | `ARRIVAL_MODEL_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `arrival_rate:sell` | Empirical observed arrival frequency for sell events. | `ARRIVAL_MODEL_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `conditional_intensity` | Model-implied instantaneous pre-arrival event intensity across marks. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `conditional_intensity:buy` | Model-implied instantaneous pre-arrival event intensity for buy events. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `conditional_intensity:sell` | Model-implied instantaneous pre-arrival event intensity for sell events. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `background_rate` | Fitted immigrant/background arrival rate absent retained excitation. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `background_rate:buy` | Fitted immigrant/background arrival rate absent retained excitation for buy events. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `background_rate:sell` | Fitted immigrant/background arrival rate absent retained excitation for sell events. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_intensity:buy` | Current buy intensity attributable to excitation above background. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | Category:Frenzy | `KEEP_NAMED_USE` |
| `excitation_intensity:sell` | Current sell intensity attributable to excitation above background. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | Category:Frenzy | `KEEP_NAMED_USE` |
| `excitation_fraction:buy` | Fraction of current fitted buy intensity attributable to prior-event excitation; model decomposition, not causal probability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | advisor:historical | `KEEP_NAMED_USE` |
| `excitation_fraction:sell` | Fraction of current fitted sell intensity attributable to prior-event excitation; model decomposition, not causal probability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_amplitude:buy_from_buy` | Instantaneous target-intensity jump caused by the named source mark in the fitted Hawkes kernel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_amplitude:buy_from_sell` | Instantaneous target-intensity jump caused by the named source mark in the fitted Hawkes kernel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_amplitude:sell_from_buy` | Instantaneous target-intensity jump caused by the named source mark in the fitted Hawkes kernel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_amplitude:sell_from_sell` | Instantaneous target-intensity jump caused by the named source mark in the fitted Hawkes kernel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_decay:buy_from_buy` | Exponential excitation decay rate for the named target/source kernel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_decay:buy_from_sell` | Exponential excitation decay rate for the named target/source kernel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_decay:sell_from_buy` | Exponential excitation decay rate for the named target/source kernel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_decay:sell_from_sell` | Exponential excitation decay rate for the named target/source kernel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_timescale:buy_from_buy` | E-folding duration (1/beta) for the named target/source excitation kernel; useful as an externally derived temporal scale. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_timescale:buy_from_sell` | E-folding duration (1/beta) for the named target/source excitation kernel; useful as an externally derived temporal scale. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_timescale:sell_from_buy` | E-folding duration (1/beta) for the named target/source excitation kernel; useful as an externally derived temporal scale. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `excitation_timescale:sell_from_sell` | E-folding duration (1/beta) for the named target/source excitation kernel; useful as an externally derived temporal scale. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `offspring:buy_from_buy` | Expected direct descendant events for the named target/source kernel (alpha/beta). | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `offspring:buy_from_sell` | Expected direct descendant events for the named target/source kernel (alpha/beta). | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `offspring:sell_from_buy` | Expected direct descendant events for the named target/source kernel (alpha/beta). | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `offspring:sell_from_sell` | Expected direct descendant events for the named target/source kernel (alpha/beta). | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `branching_spectral_radius` | Spectral radius of the Hawkes branching matrix; theorem-defined process stability geometry, not market danger. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | Category:Turbulent,VerticalIgnition | `KEEP_NAMED_USE` |
| `expected_descendants_from_buy` | Expected total future descendants under the fitted stationary branching model, excluding the ancestor; undefined at/above non-stationary boundary. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `expected_descendants_from_sell` | Expected total future descendants under the fitted stationary branching model, excluding the ancestor; undefined at/above non-stationary boundary. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `log_likelihood:hawkes` | Hawkes/model-fit likelihood or gain diagnostic; assesses model explanation versus alternatives, not direction or profitability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `log_likelihood:poisson` | Hawkes/model-fit likelihood or gain diagnostic; assesses model explanation versus alternatives, not direction or profitability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `log_likelihood:self_only` | Hawkes/model-fit likelihood or gain diagnostic; assesses model explanation versus alternatives, not direction or profitability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `log_likelihood_gain_vs_poisson` | Hawkes/model-fit likelihood or gain diagnostic; assesses model explanation versus alternatives, not direction or profitability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `log_likelihood_gain_vs_self_only` | Hawkes/model-fit likelihood or gain diagnostic; assesses model explanation versus alternatives, not direction or profitability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `log_likelihood_per_event:hawkes` | Hawkes/model-fit likelihood or gain diagnostic; assesses model explanation versus alternatives, not direction or profitability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `log_likelihood_gain_per_event_vs_poisson` | Hawkes/model-fit likelihood or gain diagnostic; assesses model explanation versus alternatives, not direction or profitability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `log_likelihood_gain_per_event_vs_self_only` | Hawkes/model-fit likelihood or gain diagnostic; assesses model explanation versus alternatives, not direction or profitability. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `conditional_intensity_velocity` | Model-implied instantaneous pre-arrival event intensity across marks. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `spectral_radius_velocity` | Rate of change of fitted branching spectral radius. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `compensator:buy` | Integrated fitted event intensity for the buy channel over the interval. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `compensator:sell` | Integrated fitted event intensity for the sell channel over the interval. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `count_innovation:buy` | Observed minus compensator event-count residual for the buy channel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `count_innovation:sell` | Observed minus compensator event-count residual for the sell channel. | `ARRIVAL_MODEL_STATE` | Requires a usable/identified Hawkes fit or compensator where applicable. Fit diagnostics/uncertainty affect whether downstream valuation may rely on it. | — | `UNMAPPED_REVIEW` |
| `standardized_innovation:buy` | Noise-standardized point-process residual for the buy channel; model-surprise diagnostic. | `ARRIVAL_MODEL_STATE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `UNMAPPED_REVIEW` |
| `standardized_innovation:sell` | Noise-standardized point-process residual for the sell channel; model-surprise diagnostic. | `ARRIVAL_MODEL_STATE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `UNMAPPED_REVIEW` |

### 9.7 `leadlag` — 29 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `last_price` | Current price sample retained to form the focal return path. | `RELATIONSHIP_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `observation_count` | Retained focal price support; provenance only. | `RELATIONSHIP_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `contemporaneous_correlation` | Return dependence at zero lag. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | Category:SynchronizedDrift | `KEEP_NAMED_USE` |
| `best_lag_correlation` | Strongest measured correlation over the declared lag search domain. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `best_lag_index` | Index of the selected lag within the search grid; provenance, not economic lag itself. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `best_lag_seconds` | Temporal offset at the strongest measured correlation; precedence only, not causality/leadership. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `absolute_correlation_gain` | Improvement in \|correlation\| from temporal shifting over zero-lag alignment. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `lag_fraction` | Selected lag relative to search span; search-domain provenance, not universal strength score. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | Category:DecoupledMove | `KEEP_REVIEW_CURRENT_USE` |
| `lag_search_resolution_seconds` | Time resolution of the lag search; inference provenance. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `lag_search_span` | Total lag domain searched; inference provenance. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `reference_return_count` | Return observations supporting the reference path. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `measured_return_count` | Return observations supporting the measured path. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `overlap_pair_count` | Number of paired returns available to lag estimation. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `effective_sample_count` | Effective independent support for correlation/lag inference. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `search_count` | Number of lag candidates searched; multiple-search provenance. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `lag_peak_prominence` | How distinctly the selected lag peak rises above neighboring lag correlations. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `lag_peak_curvature` | Local sharpness of the selected lag peak; broad plateaus are less temporally specific. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `correlation_p_value` | Inferential p-value for selected lag correlation under stated assumptions. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `search_adjusted_p_value` | P-value adjusted for the lag search multiplicity under stated assumptions. | `ESTIMABILITY` | Controls estimability/uncertainty of an inference. Never directional evidence and never a hand-tuned weight. | — | `UNMAPPED_REVIEW` |
| `lag_baseline_seconds` | Causal historical reference lag for this relation. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `lag_divergence_seconds` | Current lag departure from historical lag reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `lag_noise_scale_seconds` | Historical residual noise scale of lag estimates. | `RELATIONSHIP_STATE` | Requires temporally valid path overlap/support appropriate to the statistic; missing support is not zero relationship. | — | `UNMAPPED_REVIEW` |
| `lag_zscore` | Standardized unusualness of the current lag. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:DecoupledMove | `KEEP_NAMED_USE` |
| `lag_velocity` | Rate at which the estimated lag is moving. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `correlation_gain_baseline` | Historical reference for the benefit of temporal shifting. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `correlation_gain_zscore` | Standardized unusualness of current correlation gain. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:InefficientLag | `KEEP_NAMED_USE` |
| `correlation_gain_velocity` | Rate of change of temporal-shift correlation gain. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `best_lag_correlation_baseline` | Historical reference for best-lag correlation. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `best_lag_correlation_zscore` | Standardized unusualness of best-lag correlation. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:AnchorStall | `KEEP_NAMED_USE` |

### 9.8 `liquidity` — 11 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `best_bid_price` | Current executable best bid price; direct touch price fact. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `best_ask_price` | Current executable best ask price; direct touch price fact. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `touch_quantity:bid` | Displayed base quantity at best bid. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `touch_quantity:ask` | Displayed base quantity at best ask. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `touch_notional:bid` | Displayed quote notional at best bid. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `touch_notional:ask` | Displayed quote notional at best ask. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `midpoint` | Current bid/ask midpoint; reference geometry, not executable price for a non-trivial position. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `spread` | Absolute best-ask minus best-bid spread. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `relative_spread` | Spread divided by midpoint; scale-free touch transaction-cost geometry. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:ExtremeScarcity; advisor:liquidity | `KEEP_NAMED_USE` |
| `two_sided_touch_notional` | Minimum of bid and ask touch notionals; conservative two-sided touch capacity. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:RobustLiquidity | `KEEP_NAMED_USE` |
| `touch_notional_imbalance` | Scale-free asymmetry of bid vs ask touch notional. | `EXECUTION_CONTEXT` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:ExtremeScarcity,RobustLiquidity; advisor:liquidity | `KEEP_REVIEW_CURRENT_USE` |

### 9.9 `morphology` — 7 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `book_shape_distance` | Wasserstein-style distance between folded bid/ask normalized depth distributions; average bilateral shape mismatch. | `CONTEXT_INPUT` | Current shape facts are stateless once a valid two-sided book exists; morphology_change additionally requires a prior comparable shape. SNR is not intrinsic. | — | `KEEP_NEEDS_COMPOSITION` |
| `book_shape_ks` | Maximum cumulative discrepancy between folded bid/ask normalized depth distributions; worst local bilateral mismatch. | `CONTEXT_INPUT` | Current shape facts are stateless once a valid two-sided book exists; morphology_change additionally requires a prior comparable shape. SNR is not intrinsic. | — | `KEEP_NEEDS_COMPOSITION` |
| `concentration:bid` | Herfindahl concentration of normalized bid-side depth weights. | `CONTEXT_INPUT` | Current shape facts are stateless once a valid two-sided book exists; morphology_change additionally requires a prior comparable shape. SNR is not intrinsic. | — | `KEEP_NEEDS_COMPOSITION` |
| `concentration:ask` | Herfindahl concentration of normalized ask-side depth weights. | `CONTEXT_INPUT` | Current shape facts are stateless once a valid two-sided book exists; morphology_change additionally requires a prior comparable shape. SNR is not intrinsic. | — | `KEEP_NEEDS_COMPOSITION` |
| `entropy:bid` | Shannon entropy of normalized bid-side depth weights; spread of displayed size across levels. | `CONTEXT_INPUT` | Current shape facts are stateless once a valid two-sided book exists; morphology_change additionally requires a prior comparable shape. SNR is not intrinsic. | — | `KEEP_NEEDS_COMPOSITION` |
| `entropy:ask` | Shannon entropy of normalized ask-side depth weights; spread of displayed size across levels. | `CONTEXT_INPUT` | Current shape facts are stateless once a valid two-sided book exists; morphology_change additionally requires a prior comparable shape. SNR is not intrinsic. | — | `KEEP_NEEDS_COMPOSITION` |
| `morphology_change` | Distributional distance between current and prior normalized whole-book shape; structural change magnitude, not direction or intent. | `CONTEXT_INPUT` | Current shape facts are stateless once a valid two-sided book exists; morphology_change additionally requires a prior comparable shape. SNR is not intrinsic. | — | `KEEP_NEEDS_COMPOSITION` |

### 9.10 `pumpdump` — 37 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `trade_price` | Execution price of the current trade. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `trade_quantity` | Base quantity of the current trade. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `trade_notional` | Economic quote notional of the current trade. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `trade_interval_seconds` | Elapsed time since the prior trade; direct event-cadence fact. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:OrganicTrend | `KEEP_NAMED_USE` |
| `volume_bar_target_quantity` | Data-derived base-quantity target that closes the volume-clock observation bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `volume_bar_quantity` | Base quantity accumulated in the completed volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:VerticalIgnition,CoiledCompression | `KEEP_NAMED_USE` |
| `volume_bar_notional` | Economic notional accumulated in the completed volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `volume_bar_trade_count` | Execution count accumulated in the completed volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `volume_bar_duration` | Event-time duration required to complete the volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `volume_rate` | Completed base quantity per second over the volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:VerticalIgnition | `KEEP_NAMED_USE` |
| `notional_rate` | Completed economic throughput per second over the volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `trade_rate` | Completed execution count per second over the volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `notional_rate_velocity` | Event-time rate of change of the underlying `notional_rate_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `notional_rate_baseline` | Causal historical reference for `notional_rate`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `notional_rate_ratio` | Scale-free ratio form of the underlying `notional_rate_ratio` quantity for contextual/historical comparison. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `notional_rate_divergence` | Current departure of the underlying `notional_rate_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `notional_rate_zscore` | Standardized historical unusualness of the underlying `notional_rate_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:VerticalIgnition | `KEEP_NAMED_USE` |
| `midpoint:from` | Midpoint at start of the completed volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `midpoint:at` | Midpoint at end of the completed volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `midpoint` | Touch-price/spread context associated with the volume-clock activity observation. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `midpoint_log_return` | Midpoint log return over the completed volume-clock bar. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `midpoint_return_rate` | Completed-bar midpoint response per second. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `positive_midpoint_return` | Positive component of completed-bar midpoint return; decomposition fact, not bullishness. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `negative_midpoint_return` | Negative component of completed-bar midpoint return; decomposition fact, not bearishness. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `midpoint_return_baseline` | Causal historical reference for `midpoint_return`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `midpoint_return_divergence` | Current departure of the underlying `midpoint_return_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `midpoint_return_zscore` | Standardized historical unusualness of the underlying `midpoint_return_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:OrganicTrend,FadedExhaustion | `KEEP_NAMED_USE` |
| `midpoint_return_velocity` | Event-time rate of change of the underlying `midpoint_return_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `best_bid` | Touch-price/spread context associated with the volume-clock activity observation. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `best_ask` | Touch-price/spread context associated with the volume-clock activity observation. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `spread` | Touch-price/spread context associated with the volume-clock activity observation. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `relative_spread` | Touch-price/spread context associated with the volume-clock activity observation. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | Category:CoiledCompression | `KEEP_NAMED_USE` |
| `relative_spread_baseline` | Causal historical reference for `relative_spread`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `spread_ratio` | Scale-free ratio form of the underlying `spread_ratio` quantity for contextual/historical comparison. | `ACTIVITY_STATE` | Direct/derived fact remains valid when its stated source observations are valid; historical Maturity is not required unless the metric itself depends on history. | — | `UNMAPPED_REVIEW` |
| `spread_divergence` | Current departure of the underlying `spread_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `spread_zscore` | Standardized historical unusualness of the underlying `spread_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:CoiledCompression | `KEEP_NAMED_USE` |
| `spread_divergence_velocity` | Current departure of the underlying `spread_divergence_velocity` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |

### 9.11 `sentiment` — 63 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `return` | Focal symbol common-horizon log return within the explicit cohort snapshot. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `absolute_return` | Magnitude of focal common-horizon return. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `advance_count` | Valid cohort members with positive return. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `decline_count` | Valid cohort members with negative return. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `unchanged_count` | Valid cohort members with zero return. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `valid_member_count` | Members with valid comparable observations in this snapshot. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `cohort_member_count` | Configured cohort size; population provenance. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `excluded_member_count` | Configured members excluded for missing/stale/incomparable observations. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `same_direction_peer_count` | Peers moving in the focal return direction. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `opposite_direction_peer_count` | Peers moving opposite the focal return direction. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `zero_return_peer_count` | Peers with zero return. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `advance_fraction` | Fraction of valid members with positive return. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | Category:SystemicSlump | `KEEP_REVIEW_CURRENT_USE` |
| `decline_fraction` | Fraction of valid members with negative return. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `unchanged_fraction` | Fraction of valid members with zero return. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `directional_participation` | Fraction of valid members moving non-zero in either direction. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `directional_agreement` | Share of directional members on the majority side; direction-neutral agreement magnitude. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | Category:RiskOnSurge | `KEEP_NAMED_USE` |
| `directional_consensus` | Absolute positive-vs-negative participation imbalance among directional members. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | Category:RiskOnSurge | `KEEP_NAMED_USE` |
| `same_direction_peer_fraction` | Fraction of peers moving with focal direction. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `opposite_direction_peer_fraction` | Fraction of peers moving opposite focal direction. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `zero_return_peer_fraction` | Fraction of peers unchanged. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `breadth` | Signed cross-sectional participation balance; cohort movement breadth, not bullishness label. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `breadth_baseline` | Causal historical reference for `breadth`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `breadth_divergence` | Current departure of the underlying `breadth_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `breadth_velocity` | Event-time rate of change of the underlying `breadth_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `breadth_zscore` | Standardized historical unusualness of the underlying `breadth_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:SystemicSlump | `KEEP_NAMED_USE` |
| `median_return` | Typical signed member return in the explicit cohort. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `median_return_baseline` | Causal historical reference for `median_return`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `median_return_divergence` | Current departure of the underlying `median_return_divergence` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `median_return_velocity` | Event-time rate of change of the underlying `median_return_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `median_return_zscore` | Standardized historical unusualness of the underlying `median_return_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `UNMAPPED_REVIEW` |
| `median_absolute_return` | Typical movement magnitude in the cohort. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `median_absolute_return_baseline` | Causal historical reference for `median_absolute_return`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `median_absolute_return_ratio` | Scale-free ratio form of the underlying `median_absolute_return_ratio` quantity for contextual/historical comparison. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `median_absolute_return_velocity` | Event-time rate of change of the underlying `median_absolute_return_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `median_absolute_return_zscore` | Standardized historical unusualness of the underlying `median_absolute_return_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:DivergentMove | `KEEP_NAMED_USE` |
| `mean_absolute_return` | Mean movement magnitude across valid members. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `rms_return` | RMS movement magnitude; sensitive to larger movers. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `return_interquartile_range` | Robust spread of signed member returns. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `return_dispersion_baseline` | Causal historical reference for `return_dispersion`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `return_dispersion_ratio` | Scale-free ratio form of the underlying `return_dispersion_ratio` quantity for contextual/historical comparison. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `return_dispersion_velocity` | Event-time rate of change of the underlying `return_dispersion_velocity` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `return_dispersion_zscore` | Standardized historical unusualness of the underlying `return_dispersion_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:DivergentMove | `KEEP_NAMED_USE` |
| `return_mad` | Median absolute deviation of signed returns. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `magnitude_mad` | Median absolute deviation of return magnitudes. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `largest_absolute_return` | Magnitude of the unique/tied largest mover. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `largest_move_tie_count` | Tie provenance for the largest absolute move; determines whether unique-mover fields are defined. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `largest_move_excess` | Largest move magnitude minus peer median magnitude. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `largest_move_mad_excess` | Largest move excess expressed in peer MAD units. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `largest_signed_return` | Signed return of a unique largest mover. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `largest_move_ratio` | Largest move magnitude relative to peer median magnitude. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `largest_move_ratio_baseline` | Causal historical reference for `largest_move_ratio`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `largest_move_ratio_zscore` | Standardized historical unusualness of the underlying `largest_move_ratio_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `UNMAPPED_REVIEW` |
| `largest_move_share` | Largest mover's share of aggregate absolute movement. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `largest_move_share_baseline` | Causal historical reference for `largest_move_share`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `largest_move_share_zscore` | Standardized historical unusualness of the underlying `largest_move_share_zscore` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `UNMAPPED_REVIEW` |
| `peer_median_absolute_return` | Typical absolute movement among peers excluding the focal/extreme member. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `peer_magnitude_mad` | Robust peer movement-magnitude dispersion. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `median_asof_age_seconds` | Median staleness of member as-of observations. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `max_asof_age_seconds` | Worst staleness of member as-of observations. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `median_from_age_seconds` | Median age of member interval start points. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `cohort_horizon_seconds` | Common comparison horizon actually represented by the snapshot. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `asof_age_seconds` | Focal observation staleness within the snapshot. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |
| `from_age_seconds` | Age of focal comparison interval start. | `CROSS_SECTIONAL_CONTEXT` | Requires an explicit cohort and valid common-horizon/as-of membership. Cohort composition/age is part of the fact. | — | `UNMAPPED_REVIEW` |

### 9.12 `toxicity` — 59 metrics

| Metric | What it is for | Role | Quality / definedness | Current named use | Normative status |
|---|---|---|---|---|---|
| `best_price:bid` | Current best bid price used for touch-disposition attribution. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `best_price:ask` | Current best ask price used for touch-disposition attribution. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `previous_best_price:bid` | Prior comparable best bid price anchoring the disposition bracket. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `previous_best_price:ask` | Prior comparable best ask price anchoring the disposition bracket. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_quantity:bid` | Current displayed touch quantity on the bid side. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_quantity:ask` | Current displayed touch quantity on the ask side. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `previous_touch_quantity:bid` | Previous displayed touch quantity on the bid side, denominator for disposition fractions. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `previous_touch_quantity:ask` | Previous displayed touch quantity on the ask side, denominator for disposition fractions. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_price_log_change:bid` | Log movement of the bid touch price across the attribution bracket. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_price_log_change:ask` | Log movement of the ask touch price across the attribution bracket. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `unfilled_residual_quantity:bid` | Previous bid touch quantity not explained by matched execution before disposition accounting. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `unfilled_residual_quantity:ask` | Previous ask touch quantity not explained by matched execution before disposition accounting. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_withdrawn_quantity:bid` | Unexplained displayed quantity removed from the previous bid touch after matched fills. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_withdrawn_quantity:ask` | Unexplained displayed quantity removed from the previous ask touch after matched fills. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_replenished_quantity:bid` | Displayed quantity restored/added at the same bid touch after disposition. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_replenished_quantity:ask` | Displayed quantity restored/added at the same ask touch after disposition. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `retreated_quantity:bid` | Quantity associated with bid touch retreat away from the prior price. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `retreated_quantity:ask` | Quantity associated with ask touch retreat away from the prior price. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_withdrawal_fraction:bid` | Fraction of previous bid touch removed without matched execution. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_withdrawal_fraction:ask` | Fraction of previous ask touch removed without matched execution. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_replenishment_fraction:bid` | Same-price replenishment relative to previous bid touch quantity. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_replenishment_fraction:ask` | Same-price replenishment relative to previous ask touch quantity. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `retreat_fraction:bid` | Fractional bid touch retreat state. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `retreat_fraction:ask` | Fractional ask touch retreat state. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_withdrawal_rate:bid` | Unexplained withdrawal rate on the bid touch. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_withdrawal_rate:ask` | Unexplained withdrawal rate on the ask touch. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_replenishment_rate:bid` | Same-price replenishment rate on the bid side. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `net_replenishment_rate:ask` | Same-price replenishment rate on the ask side. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `retreat_rate:bid` | Rate of bid touch retreat. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `retreat_rate:ask` | Rate of ask touch retreat. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `withdrawal_fraction_baseline:bid` | Causal historical reference for `withdrawal_fraction`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `withdrawal_fraction_baseline:ask` | Causal historical reference for `withdrawal_fraction`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `withdrawal_fraction_divergence:bid` | Current departure of the underlying `withdrawal_fraction_divergence:bid` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `withdrawal_fraction_divergence:ask` | Current departure of the underlying `withdrawal_fraction_divergence:ask` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `withdrawal_fraction_zscore:bid` | Standardized historical unusualness of the underlying `withdrawal_fraction_zscore:bid` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:ToxicBluff | `KEEP_REVIEW_CURRENT_USE` |
| `withdrawal_fraction_zscore:ask` | Standardized historical unusualness of the underlying `withdrawal_fraction_zscore:ask` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:ToxicBluff | `KEEP_REVIEW_CURRENT_USE` |
| `withdrawal_fraction_velocity:bid` | Event-time rate of change of the underlying `withdrawal_fraction_velocity:bid` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `withdrawal_fraction_velocity:ask` | Event-time rate of change of the underlying `withdrawal_fraction_velocity:ask` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `retreat_fraction_baseline:bid` | Causal historical reference for `retreat_fraction`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `retreat_fraction_baseline:ask` | Causal historical reference for `retreat_fraction`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `retreat_fraction_zscore:bid` | Standardized historical unusualness of the underlying `retreat_fraction_zscore:bid` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `UNMAPPED_REVIEW` |
| `retreat_fraction_zscore:ask` | Standardized historical unusualness of the underlying `retreat_fraction_zscore:ask` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | — | `UNMAPPED_REVIEW` |
| `bracket_trade_quantity` | Total executed quantity observed in the attribution bracket. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `matched_touch_trade_quantity:bid` | Trade quantity exactly attributable to the previous bid touch price. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `matched_touch_trade_quantity:ask` | Trade quantity exactly attributable to the previous ask touch price. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_fill_quantity:bid` | Previous bid touch quantity accounted for by matched executions. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_fill_quantity:ask` | Previous ask touch quantity accounted for by matched executions. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_fill_fraction:bid` | Fraction of previous bid touch quantity accounted for by executions. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_fill_fraction:ask` | Fraction of previous ask touch quantity accounted for by executions. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_fill_rate:bid` | Execution-attributed depletion rate of the previous bid touch. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `touch_fill_rate:ask` | Execution-attributed depletion rate of the previous ask touch. | `LIQUIDITY_DISPOSITION` | Requires a valid comparable touch/trade bracket for attribution; feed/order ambiguity makes attribution undefined. | — | `UNMAPPED_REVIEW` |
| `fill_fraction_baseline:bid` | Causal historical reference for `fill_fraction`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `fill_fraction_baseline:ask` | Causal historical reference for `fill_fraction`. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `fill_fraction_divergence:bid` | Current departure of the underlying `fill_fraction_divergence:bid` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `fill_fraction_divergence:ask` | Current departure of the underlying `fill_fraction_divergence:ask` quantity from its causal historical reference. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `fill_fraction_zscore:bid` | Standardized historical unusualness of the underlying `fill_fraction_zscore:bid` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:LiquidityVacuum | `KEEP_NAMED_USE` |
| `fill_fraction_zscore:ask` | Standardized historical unusualness of the underlying `fill_fraction_zscore:ask` quantity. | `CONTEXT_OR_MODEL_FEATURE` | Requires a causal baseline/noise model. Definedness and estimator support matter; magnitude is unusualness, not probability. | Category:LiquidityVacuum | `KEEP_NAMED_USE` |
| `fill_fraction_velocity:bid` | Event-time rate of change of the underlying `fill_fraction_velocity:bid` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |
| `fill_fraction_velocity:ask` | Event-time rate of change of the underlying `fill_fraction_velocity:ask` state. | `CONTEXT_OR_MODEL_FEATURE` | Requires prior causal estimator/state appropriate to the metric; undefined until that prerequisite exists. | — | `UNMAPPED_REVIEW` |

## 10. Production cleanup rule

For every projected producer identity:

```text
Is its physical/statistical meaning specified?
    no  -> SPEC_OR_DELETE; no semantic consumer may be added
    yes
      ↓
Does it have a canonical source?
    duplicate legacy -> migrate consumers, stop projecting duplicate
      ↓
Is it only derivation/support/diagnostic state?
    yes -> keep internal/kernel-local unless a named consumer truly needs it
      ↓
Does a declared cross-signal relationship or validated research result need it?
    yes -> add a TYPED semantic edge / composition
    no  -> delete projection unless a concrete downstream question is documented
```

No metric becomes 'alive' merely because `graph.Solver`, Category, an Advisor, or MCTS subscribes to all Measurements.

## 11. Immediate architecture consequences from this map

1. **Do not wire 401 red dots to MCTS.** Most information must first participate in typed relationships or coherent descriptive compositions.
2. **Do not grow `exhaustion`.** Migrate its current Category consumers to canonical Liquidity/Depthflow/CVD equivalents, then remove the duplicate signal.
3. **Close the Liquidity implementation/spec gap deliberately.** The restored spec defines historical baselines, noise, z-scores, divergence velocities and recurrence that the current 11-metric implementation does not publish. Do not add them piecemeal or duplicate Morphology ownership.
4. **Morphology's seven currently unconsumed facts are not garbage.** They have a coherent destination: Morphology Perspective and declared historical/coordination/microstructure context. They should not be shoved directly into Category/MCTS.
5. **Review Category mappings that currently leap from one measurement to a semantic label.** In particular `touch_imbalance → SpoofTrap`, `lag_fraction → DecoupledMove`, single liquidation imbalance → `ShortSqueeze`, and contradictory use of one liquidity imbalance for both scarcity and robustness.
6. **Build future Advisor/causal compositions from the typed relation catalog above.** The relationship sections in the signal specs are the authority; new combinations require a separate research result.
7. **Extend lineage eventually to display semantic edge type.** Producer→subscriber plumbing is insufficient. The useful graph is `metric → relationship/composition → question/consumer`, with support/quality edges distinct from effect-conditioning edges.

## 11.1 Liquidity spec-declared implementation gaps

The restored Liquidity specification exposes a second kind of lineage debt: **specified facts that do not exist in production yet**.

The current implementation projects 11 core touch metrics. The following Liquidity-owned families are specified but absent:

```text
touch_notional_baseline:{bid,ask}
depth_ratio:{bid,ask}
depth_divergence:{bid,ask}
depth_noise_scale:{bid,ask}
depth_zscore:{bid,ask}

relative_spread_baseline
spread_ratio
spread_divergence
spread_zscore

divergence_velocity:{bid,ask}
spread_divergence_velocity
divergence_velocity_snr:*

historical_path_distance
historical_path_percentile
historical_match_from
```

These are not "dead" because they are not producers yet. They are **contract/implementation gaps**.

The full inventory, including the morphology ownership conflict, is in `metric_spec_gaps.csv`.

## 12. Machine-readable companions

- `metric_map.csv`: one row per current projected metric with purpose, role, definedness, current named use, normative destinations, forbidden use and review status.
- `metric_relationships.csv`: typed source-level relationship catalog.
- `metric_map.json`: current per-metric catalog plus typed relationships and explicit spec gaps for tooling.
- `metric_spec_gaps.csv`: metrics declared by the restored Liquidity specification but not projected by the current implementation, including ownership conflicts with `signal/morphology`.

These files intentionally distinguish **current wiring** from **normative semantic use**. A current Category mapping is evidence of implementation, not proof that the mapping is conceptually correct.

---

## Appendix A — source counts

| Source | Current projected metric identities | Currently named in reviewed Category/Advisor declarations |
|---|---:|---:|
| `correlation` | 34 | 7 |
| `cvd` | 40 | 7 |
| `depthflow` | 35 | 7 |
| `derivatives` | 34 | 8 |
| `exhaustion` | 30 | 5 |
| `hawkes` | 53 | 5 |
| `leadlag` | 29 | 5 |
| `liquidity` | 11 | 3 |
| `morphology` | 7 | 0 |
| `pumpdump` | 37 | 7 |
| `sentiment` | 63 | 6 |
| `toxicity` | 59 | 4 |
| **TOTAL** | **432** | **64** |

The named-use count above is deliberately conservative and based on the reviewed static Category/Advisor declarations, not generic kernel subscriptions. It is therefore a semantic-use audit, not a replacement for the generated runtime lineage artifact.