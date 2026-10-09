# SYMM Empirical Audit Contract

The audit exists to measure the pipeline, not to make it pass.

## Scope

The audit is a read-only research instrument over captured market/sensory data. Audit work must not alter collector data, model checkpoints, training state, or trading state.

Changes to production learning/execution code discovered by the audit belong in separate commits/PRs from changes to the audit instrument. This keeps the thing being measured separate from the measuring instrument.

## Verdict vocabulary

Use only these meanings:

- `CONTRACT_BREACH`: a hard declared mathematical invariant was violated.
- `MEASURED`: the experiment ran and the measurements are reported. No claim of health is implied.
- `SUPPORTED`: an explicitly stated hypothesis separated from its empirical null/control under the experiment described in the report.
- `NOT_SUPPORTED`: the experiment had sufficient evidence but did not separate from its null/control.
- `INSUFFICIENT_DATA`: there was not enough evidence to evaluate the hypothesis.
- `INVALID_EXPERIMENT`: production-faithful replay or required provenance could not be established.

Do not manufacture PASS/FAIL thresholds for descriptive statistics.

## Stage 0 — mathematical contracts

Contracts come from declared metadata and explicit producer contracts, never from substrings in metric names.

Examples:
- `UnitCorrelation`: finite and in [-1, 1].
- `UnitProbability`, `UnitConfidence`: finite and in [0, 1].
- intrinsically non-negative physical units such as count, volume, duration, distance, and variance: finite and >= 0.
- `UnitZScore`, `UnitLogReturn`, velocity/acceleration, `UnitCovariance`, and signed flows/offsets (`UnitQuantity`, `UnitNotional`, `UnitSecond`): finite, otherwise unbounded unless their producer explicitly declares a tighter domain.

Never clamp an observed breach. Report the declared contract and the observation. Root-cause diagnosis is a separate investigation unless proven directly by the audit.

## Stage 1 — vitality and redundancy

Report raw producer series separately from canonical grid cells.

Coverage, variance, zero fraction and pairwise dependence are observations, not health thresholds. Absence is not zero and event-driven metrics are not defective merely because they are sparse.

Correlation alone does not prove operational redundancy. A metric is only demonstrated redundant when removing it leaves the downstream representation unchanged within the experiment being performed.

## Stage 2 — sympathy

Use the exact production transformation path:
`ChannelsFrom -> Stream.Deform`.

Missing observations remain missing. They must never be inserted as numerical zero.

For each pair, real dependence is computed only on ticks where both deformations were actually observed.

The permutation null must preserve each channel's observation mask and marginal values while destroying cross-channel alignment. Because direct and inverse sympathy are both relationships, two-sided comparisons use the distribution of `abs(null)`, not the signed upper tail.

Report the real distribution, empirical null distribution, support counts and empirical rank/separation. Do not invent a PASS cutoff.

## Stage 3 — grid reproducibility

Region balance is not evidence because balanced region cardinality is enforced by the partitioner.

The empirical question is whether independent grids recover the same co-membership structure.

Evaluate repeated disjoint chronological windows and, where data permits, multiple evidence sizes. Report agreement as a function of evidence and compare against a destroyed-data/random-partition baseline.

A single ARI value must not be converted into a health threshold.

## Stage 4 — token dynamics

A grid is developed only on the training period and evaluated on later unseen tape by invoking the production `Grid.Observe()` and `Grid.RegionScores()` path directly.

The `Stream` state is continuous across the train/holdout boundary: carrying past observations forward is causal and matches live operation.

Temporal nulls must preserve the local persistence that is not the hypothesis under test. Any block/persistence scale must be derived from the observed sequence or reported as a sensitivity sweep, not hard-coded because it appears reasonable.

Raw temporal transitions are heavily dominated by diagonal dwell time (market autocorrelation). To isolate genuine state-change dynamics, the audit must compute and report run-length compressed transition matrices alongside raw matrices, comparing compressed entropy against temporal block nulls.

## Stage 5 — precursor hypotheses

Stage 5 is event-centred, not dependent on whether an arbitrary first-N-ticks sample happens to contain a detection.

Statistical separation, predictive skill, and economic relevance must be strictly decoupled and reported independently:
1. **Statistical Separation**: Jensen-Shannon Divergence in bits of precursor token distributions ($A \to B$ ignition and $B \to C$ exhaustion) versus mask-preserving shuffled null distributions.
2. **Predictive Skill**: Out-of-sample classification performance of precursor tokens anticipating excursions (Balanced Accuracy, MCC, Precision, Recall, and Mutual Information bits) versus marginal baselines.
3. **Economic Relevance**: Net return and friction clearance rate evaluated against explicit, declared taker fee schedules ($2 \times \text{takerFee}$) and slippage. No arbitrary fee fallbacks are permitted.

For the current long-only Desk semantics:
- A->B positive examples: profitable `up` excursions.
- Controls: `up_friction`, `down`, `chop`, `flat`, plus matched non-excursion background.
- B->C is evaluated only after a valid profitable entry episode.

Retrieve the actual causal sensory windows around each detected event and tokenize event/control populations through the same production representation path. Background populations exclude the event intervals being tested.

No evidence is `INSUFFICIENT_DATA`, never success.

## Stage 6 — cognitive engine & associative memory

Stage 6 audits the cognitive engine, associative Radix Trie, and S3-compatible prefix memory (`nomagique/cognition/associate.go`, `strategy/model.go`, and `strategy/training.go`).

1. **Prequential Evaluation Protocol**:
   - Each phase context is evaluated via `Recall(context, stance)` strictly *before* it is taught (`Teach(context, class, feedback)`).
   - Ground truth actions:
     - Profitable `up` precursor: `enter`.
     - Profitable `up` holding: `exit`.
     - Controls/losing (`up_friction`, `down`, `chop`, `flat`): `wait` (abstention / do not enter; dampens `enter`).
   - Abstention (`call.Winner == ""`) is the correct stance on controls, not a terminal action.

2. **Null and Baseline Comparisons**:
   - Prequential hits and accuracy must be compared against the **best constant policy baseline** (`always_enter`, `always_exit`, `always_abstain`).
   - Compare against an empirical **label-shuffled null** across sequence fragments with fixed random seed.
   - Prequential predictive skill is `SUPPORTED` only when it separates from both the best constant policy baseline and the 95th percentile of the empirical null distribution. Otherwise, it is `NOT_SUPPORTED`.

3. **S3-Compatible Prefix Memory Audit**:
   - Audits the genuine S3 prefix-memory mechanism (`R04/R01/enter.json`) rather than an isolated in-memory trie stand-in.
   - Measures prefix collisions (`enter` vs `wait`), conflicting continuations (`enter` vs `exit`), time-to-disambiguation token depth, and prequential retrieval accuracy.
   - Enforces explicit taker fee provenance on all economic evaluations.

4. **Zero Storage Side-Effects**:
   - All models in Stage 6 must run entirely in memory. The audit must never write to Iceberg tables, SeaweedFS, S3, or SQLite.

## Adversarial Validations (V1–V4)

- **V1: Production-vs-Audit Equivalence**: Verifies that replaying tape through the audit harness produces bit-for-bit identical region token emissions and brightness scores compared to the production execution pipeline.
- **V2: Metric Truthfulness**: Verifies that sensory metrics faithfully reflect raw exchange trade measurements without non-positive prices, non-positive volumes, fake zero-filled midpoints, or synthetic time steps.
- **V3: Causality & State Isolation**: Verifies that perturbing future ticks does not alter past token emissions (zero forward leakage), and that processing multi-symbol tape preserves strict state isolation between currency pairs.
- **V4: Grid Dependence & Sensitivity**: Performs Leave-One-Family-Out (LOFO) ablation across all feature families and tests family duplication invariance to guarantee that the grid resists domination by single sensors and retains confluence.

## Reproducibility

Every audit artifact must record enough provenance to reproduce it:
- repository commit;
- epoch/data window;
- symbol/universe;
- tick/event counts;
- random seed;
- permutation count;
- transformation path/version;
- null construction.

Old audit artifacts should not be silently overwritten when comparing algorithm revisions.

## Rule for future agents

When an audit produces an uncomfortable result, do not tune the audited production component to improve the score and do not move a threshold until it turns green.

First determine which of these is true:
1. the experiment is invalid;
2. the evidence is insufficient;
3. the hypothesis is not supported;
4. a hard contract is genuinely violated.

Only then change production code, in a separate change with the measured reason recorded.
