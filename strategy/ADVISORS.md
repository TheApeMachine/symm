# Advisors, Perspectives, and Universe Discovery

> **Status:** Architecture and research specification  
> **Repository:** `TheApeMachine/symm`  
> **Baseline reviewed:** `0a86be116c72956556766d433deff495f4d68a70`  
> **Primary intent:** Give Opportunity, Planner, PositionRisk, and Hindsight richer symbol-specific and situational context without turning Measurements into decisions, Perspectives into gates, or the live runtime into an all-to-all research engine.

---

## 1. Purpose

SYMM already measures a large number of distinct properties of the market:

- order-book structure and mutation,
- executed flow,
- event-arrival excitation,
- liquidity,
- correlation,
- lead/lag,
- derivatives state,
- exhaustion,
- sentiment / cross-sectional state,
- manifold state,
- category state,
- causal relationships,
- passage outcomes,
- and other logic outputs.

Many of these observations are useful in combination, but a combination does not automatically belong inside:

- a signal,
- a graph edge,
- an opportunity gate,
- MCTS,
- or a stoploss rule.

The missing abstraction is a descriptive context layer.

This specification introduces:

```go
type Advisor struct { ... }
type Perspective struct { ... }
```

The naming is intentional.

An **Advisor** is an operational component that consumes existing observations and maintains bounded resident context.

A **Perspective** is the advisor's current descriptive output.

A Perspective tells a consumer something useful about:

- what is happening now,
- how unusual it is,
- what similar historical episodes looked like,
- how this symbol differs from the current population,
- which other symbols are currently behaving similarly,
- where a live position sits relative to historically observed paths,
- or how multiple already-measured facts relate.

A Perspective does **not** decide what to do.

The governing principle is:

> **Advisors advise. Consumers do their own research.**

Opportunity, Planner, PositionRisk, and Hindsight may all consume a Perspective, but each remains responsible for its own decision semantics.

---

## 2. Architectural Vocabulary

The terms below are distinct and MUST remain distinct.

### 2.1 Raw market events

Examples:

- L3 book updates,
- trades,
- tickers,
- executions,
- derivatives feeds.

They are venue facts.

### 2.2 Signals

Signals condition raw market events into mathematically defined Measurements.

Signals:

- measure;
- normalize;
- denoise where mathematically justified;
- preserve provenance;
- maintain causal historical baselines.

Signals MUST NOT interpret their output as a trading action.

### 2.3 Measurement

A Measurement answers:

> **What did this signal observe?**

The signal specifications already define quantities such as:

- baseline divergence,
- z-scores,
- SNR,
- maturity,
- velocity,
- recurrence distance,
- correlation,
- lag,
- liquidity,
- book geometry,
- flow,
- and related metrics.

Measurements may have many legitimate downstream consumers, including each other.

### 2.4 Category

A Category is semantic dimensionality reduction over Measurements.

It describes a present market state such as:

```text
CoiledCompression
HiddenAbsorption
InefficientLag
BookThinning
StochasticNoise
VerticalIgnition
ActiveReversal
...
```

These categorical labels are then used to observe transitions from one category into another.

These transitions are stored as sequences in a predictive Tree structure.

A Category is not automatically an opportunity or action.

### 2.5 Advisor

An Advisor combines already-produced observations into one bounded descriptive context.

It may consume:

- Measurements,
- Categories,
- Manifold output,
- Cognition output,
- Graph / relation output,
- Passage state,
- or other typed logic output.

It MUST NOT reimplement a raw signal merely to obtain the same metric again.

### 2.6 Perspective

A Perspective answers:

> **What context is relevant to the current symbol, relationship, market state, or position?**

Examples:

- "the current path closely resembles five prior episodes";
- "this pair's correlation has recently become unusually strong relative to its own baseline";
- "the current order-book morphology is a cross-sectional outlier";
- "the current adverse excursion remains inside the historical envelope of matched successful episodes";
- "the symbol is moving idiosyncratically while most of its cohort is synchronized";
- "the present liquidity decline is accelerating rather than reverting."

A Perspective describes.

It does not choose an action.

### 2.7 Opportunity

Opportunity is a typed hypothesis that an asymmetric economic situation may be forming.

It may use Perspectives as context.

It remains separate from them.

### 2.8 Planner / Strategy

Planner evaluates economic interventions.

It may consume Perspectives to better understand a candidate.

No Perspective is a generic gate.

### 2.9 PositionRisk

PositionRisk owns the risk envelope of an admitted open position.

It may consume current Perspectives to distinguish:

- ordinary adverse excursion inside a still-valid situation,
- deteriorating context,
- changing liquidity,
- or a true thesis invalidation.

PositionRisk remains constrained by the lot's admitted wallet-risk budget.

### 2.10 Hindsight

Hindsight judges what actually happened.

It uses Perspectives to determine whether:

- detection was late,
- context was misunderstood,
- a position was stopped inside an ordinary adverse excursion,
- an opportunity dissolved,
- a relationship broke,
- management failed,
- or risk control was correct.

### 2.11 Research / Discovery

Research performs exhaustive offline analysis over recorded captures.

Research may do work that is explicitly forbidden in the live runtime, including broad universe scans and all-pairs analysis.

Research produces validated descriptive models/catalogs for live Advisors.

Research does not directly place trades.

---

## 3. Core Invariants

The following statements are non-negotiable.

> **Signals measure.**

> **Advisors contextualize.**

> **Perspectives describe.**

> **Opportunity hypothesizes.**

> **Planner chooses.**

> **PositionRisk owns the live position.**

> **Hindsight judges afterward.**

And:

> **A Perspective is never a buy/sell/hold instruction.**

> **A Perspective is never a generic veto.**

> **A Perspective is never a generic opportunity score.**

> **A Perspective is never proof of causation or intent.**

---

## 4. Explicit Non-Claims

An Advisor MUST NOT claim:

```text
this symbol will pump
this symbol will fall
this pair is manipulated
this actor is a bot
this market is safe
this position should be held
this position should be sold
this opportunity is profitable
```

An Advisor MAY state defensible descriptive facts such as:

```text
the current morphology has unusually regular level spacing
the current path resembles prior episodes
the pair is currently more correlated than its own historical baseline
the strongest measured alignment occurs at a non-zero lag
the lag relationship has persisted for N observations
the current drawdown is inside/outside historical matched-episode excursions
the current symbol is a cross-sectional outlier
the current pair relationship is newly emergent
```

Intentional market manipulation may be a downstream hypothesis.

It is not an observable fact.

---

## 5. Relationship to Existing Signal Specifications

The signal specifications already contain a large part of the Advisor design.

Each signal specification describes:

1. what the signal measures;
2. its causal baselines;
3. its historical recurrence metrics;
4. its valid cross-signal relationships;
5. its explicit non-claims.

These relationship sections SHOULD be treated as the initial Advisor composition catalog.

For example, the Lead/Lag specification permits downstream comparison with:

- Correlation,
- CVD / executed flow,
- Hawkes,
- Liquidity,
- Depthflow,
- Toxicity,
- cross-sectional state,
- and Derivatives.

The Lead/Lag signal itself does not interpret those combinations.

That interpretation belongs naturally in Advisors.

### Rule

> **Advisor composition MUST be justified by declared signal relationships or a separate research result.**

An Advisor MUST NOT arbitrarily combine metrics because they "look useful."

---

# Part I — Live Advisor Architecture

## 6. Advisor Operational Contract

Conceptually:

```go
type Advisor struct {
    // bounded resident state
}
```

The exact implementation may use concrete solver types rather than one universal struct, but all Advisors obey the same contract.

An Advisor:

1. subscribes to one or more typed streams;
2. consumes each event once;
3. mutates bounded resident state;
4. optionally emits a Perspective;
5. retains no unbounded event backlog;
6. never reconstructs a world snapshot to process one event.

The live flow remains:

```text
event
  ↓
measurement / logic output
  ↓
Advisor.Step(...)
  ↓
mutate resident context
  ↓
Perspective
  ↓
done
```

Not:

```text
event
  ↓
collect all symbols
  ↓
snapshot world
  ↓
clone histories
  ↓
sort everything
  ↓
derive context
```

---

## 7. Perspective Contract

A `Perspective` is a small, typed, streamable description of current context.

Conceptually:

```go
type Perspective struct {
    Symbol   string
    Peer     string // optional; empty for symbol/global perspectives

    Kind     PerspectiveKind
    From     time.Time
    At       time.Time
    Sequence uint64

    Maturity float64

    // Kind-specific, fixed-size payload.
    Payload PerspectivePayload
}
```

This is conceptual, not an instruction to implement a large union exactly this way.

### 7.1 Required implementation properties

The hot-path representation MUST:

- be fixed-size or tightly bounded;
- use typed numeric/interned identities;
- avoid `map[string]any`;
- avoid arbitrary string lists;
- avoid per-event slices;
- avoid snapshots;
- avoid clones;
- avoid rendered string identity;
- avoid unbounded provenance;
- avoid `decimal.Decimal` in computational hot paths unless venue arithmetic requires it.

### 7.2 No generic confidence score

A Perspective MUST NOT expose a scalar such as:

```text
confidence = 0.92
perspective_strength = 0.81
```

unless that value has a precise mathematical definition.

Prefer the actual quantities:

```text
effective sample count
distance
percentile
lag seconds
correlation
correlation gain
SNR
duration
episode count
excursion quantile
cross-sectional percentile
```

### 7.3 Definedness

Missing information is missing.

Do not turn:

```text
no historical analogue model
```

into:

```text
distance = 0
```

Every optional metric needs explicit definedness.

---

## 8. Perspective Identity

A Perspective is identified by a bounded structural key.

Conceptually:

```text
(Symbol, Kind)
(Symbol, Peer, Kind)
(PositionID, Kind)
(GlobalScope, Kind)
```

The identity MUST remain structural.

Do not build string IDs in comparators or hot loops.

---

## 9. Perspective Delivery

Perspectives are current context, not transport history.

Old Perspectives are normally superseded by newer Perspectives of the same key.

Therefore:

- UI delivery SHOULD use `DeliveryLatestByKey`;
- live analytical consumers MAY use observational FIFO when every transition matters;
- PositionRisk MUST NOT synchronously wait for an Advisor before processing a price mark;
- a stale/unavailable Perspective MUST degrade to less context, not block the PositionGuardian.

If PositionRisk consumes Advisor context, the latest available state MUST be locally readable without causing the priority guardian to enter the analytics scheduler.

### Safety invariant

> **Risk protection never waits for analytics.**

The absolute wallet-loss boundary remains independent of Advisor availability.

---

# Part II — Perspective Families

## 10. Historical Analogue Perspective

### 10.1 Question

> **Has this symbol previously exhibited a trajectory similar to the one observed now?**

And:

> **Where does the present trajectory sit relative to those historical episodes?**

### 10.2 Inputs

May include standardized trajectories from:

- Category,
- Hawkes,
- CVD,
- Depthflow,
- Liquidity,
- Toxicity,
- Derivatives,
- PumpDump,
- Manifold,
- and other stable typed outputs.

It SHOULD use the smallest sufficient state vector for the research question.

### 10.3 Outputs

Examples:

```text
matched episode count
nearest historical path distance
nearest historical path percentile
nearest match start/end
current alignment offset within the matched path
current elapsed duration
matched episode duration distribution
current adverse excursion
historical adverse-excursion distribution for matched episodes
historical favorable-excursion distribution for matched episodes
historical observed terminal states
```

### 10.4 Important semantic distinction

It may say:

```text
5 close historical analogues were found;
3 later exhibited VerticalIgnition;
2 dissolved without ignition.
```

That is a statement about recorded history.

It MUST NOT automatically transform this into:

```text
P(VerticalIgnition) = 0.60
```

unless a separately calibrated probability model justifies that interpretation.

### 10.5 Stage alignment

The current episode SHOULD be alignable to historical analogues.

Example:

```text
current path is closest to the 35–50% region
of historically matched episode duration
```

This is particularly useful for:

- precursor maturity,
- live position tolerance,
- post-entry drawdown interpretation,
- and Hindsight.

---

## 11. Coordination / Coattail Perspective

### 11.1 Question

> **Which instruments are currently behaving together in a way that is unusual relative to their ordinary relationship?**

The internal research theme may be called:

> **Riding the Coattails**

The Advisor MUST remain descriptive.

### 11.2 Why relationship onset matters

The interesting fact is often not:

```text
DENT and CAMP are correlated.
```

It is:

```text
DENT and CAMP were weakly related,
then a strong local relationship emerged.
```

Relationship **change** is often more informative than full-history average dependence.

### 11.3 Core inputs

Use existing Correlation and Lead/Lag quantities where available:

```text
contemporaneous correlation
best-lag correlation
best-lag seconds
absolute correlation gain
lag peak prominence
lag peak curvature
pair SNR
pair historical-path percentile
lag z-score
correlation-gain z-score
lag velocity
correlation-gain velocity
```

Additional contextual inputs may include:

- volume / notional trajectories,
- Hawkes arrival/excitation state,
- liquidity divergence,
- executed flow,
- morphology,
- Category trajectories.

### 11.4 Outputs

Example:

```text
peer = DENT/USD

ordinary relationship:
    historical correlation baseline
    historical lag baseline

current relationship:
    contemporaneous correlation
    best-lag correlation
    best-lag seconds
    absolute correlation gain

relationship novelty:
    current correlation divergence
    current lag divergence
    historical path percentile

onset:
    first observation where current coupling episode became distinguishable

persistence:
    current episode duration

lead/lag:
    observed temporal ordering if statistically supported

shared dimensions:
    price
    volume
    liquidity
    flow
    morphology
```

### 11.5 No manipulation claim

The Perspective may report:

```text
unusually synchronized
repeatedly reconstructed
stable temporal precedence
multi-signal coordination
```

It MUST NOT report:

```text
manipulated
same operator
same bot
coordinated criminal activity
```

Those are causal/intentional hypotheses unsupported by market telemetry alone.

---

## 12. Book Morphology Perspective

### 12.1 Question

> **How is liquidity arranged, and how unusual is that arrangement relative to this symbol and the current market population?**

### 12.2 Dimensionless normalization

Absolute book depth is not cross-market comparable.

Normalize level distance:

\[
r_i = \frac{|p_i - mid|}{spread}
\]

and level weight:

\[
w_i = \frac{p_i q_i}{\sum_j p_j q_j}
\]

This converts very different price scales into comparable book-shape distributions.

### 12.3 Structural measurements

Useful measured properties include:

```text
depth concentration
normalized entropy
depth center of mass
depth dispersion
bid/ask shape divergence
level-spacing regularity
size quantization
shape persistence
shape turnover
replenishment cadence
cancellation cadence
```

Useful established measures include:

Herfindahl concentration:

\[
H = \sum_i w_i^2
\]

Normalized entropy:

\[
E =
-\frac{\sum_i w_i \log w_i}{\log n}
\]

Bid/ask shape difference MAY use Jensen–Shannon divergence.

### 12.4 Cross-sectional output

The Advisor may say:

```text
most current books:
    high entropy
    low spacing regularity
    low size quantization

this symbol:
    entropy percentile = ...
    spacing regularity percentile = ...
    quantization percentile = ...
    reconstruction persistence percentile = ...
```

It should not say "bot" or "organic" as a measured fact.

Those may be downstream descriptive hypotheses.

---

## 13. Relative-State / Cross-Sectional Perspective

### 13.1 Question

> **How does this symbol differ from the current market/cohort population?**

Examples:

```text
most symbols are moving together;
this symbol is decoupled.

most books have ordinary morphology;
this symbol is an extreme outlier.

most symbols have stable liquidity;
this symbol's ask depth is collapsing.

most symbols are in ordinary arrival regimes;
this symbol's Hawkes state is exceptional.
```

### 13.2 Construction

Cross-sectional context SHOULD use comparable standardized quantities.

Useful population statistics:

```text
median
MAD / robust dispersion
quantiles
breadth
cross-sectional correlation
cross-sectional entropy
cross-sectional covariance where defensible
```

Do not compare raw quantities whose units or scales are incompatible.

---

## 14. Liquidity Perspective

### 14.1 Question

> **What terrain is this symbol or position currently trading through?**

### 14.2 Inputs

Use existing liquidity/depth measurements:

```text
depth ratio
depth divergence
depth z-score
divergence velocity
spread
spread divergence
touch capacity
impact
liquidity SNR
historical path distance
historical path percentile
```

### 14.3 Outputs

A Perspective may distinguish:

```text
thin but stable liquidity
rapidly deteriorating bid support
reverting liquidity vacuum
spread expansion without depth loss
depth loss with stable spread
unusually hostile execution terrain
```

These descriptions must be grounded in measured components.

### 14.4 Position use

PositionRisk may use this Perspective to understand whether an adverse mark occurred:

- in deep ordinary liquidity,
- through a temporary vacuum,
- during rapidly changing spread/impact,
- or under a structurally different execution regime.

It still must respect the admitted wallet risk budget.

---

## 15. Flow Perspective

### 15.1 Question

> **What are actual market participants and event processes doing around the current price move?**

### 15.2 Inputs

Potential inputs:

- CVD,
- toxicity,
- Hawkes,
- quote retreat,
- absorption,
- depth mutation,
- replenishment.

### 15.3 Example contrast

The same price decline may occur with:

```text
price falling
aggressive accumulation remains elevated
ask liquidity is withdrawing
```

or:

```text
price falling
sell aggression is accelerating
bid liquidity is retreating
```

These are different contexts.

The Advisor describes the difference.

It does not decide whether to hold or sell.

---

## 16. Passage / Position Perspective

### 16.1 Question

> **Where is this live position relative to historically observed paths of comparable positions or opportunity episodes?**

### 16.2 Inputs

Use:

```text
current MAE
current MFE
position age
entry opportunity archetype
entry opportunity phase
current opportunity phase
matched analogue state
historical first-passage results
historical adverse excursion
historical favorable excursion
historical time-to-resolution
```

### 16.3 Outputs

Examples:

```text
current adverse excursion is ordinary/unusual relative to matched episodes
current age exceeds most successful matched episodes
similar successful episodes commonly experienced deeper MAE
current path has diverged from successful historical analogues
profit boundary has historically followed comparable states
```

Again, this is context.

PositionRisk owns the decision.

---

# Part III — Consumers

## 17. Opportunity Consumption

Opportunity may consume Perspectives to add nuance to a candidate.

Example:

```text
Category:
    CoiledCompression

HistoricalAnalogue:
    current trajectory resembles prior ignition precursors

Coordination:
    symbol is currently inside a newly emergent synchronized cluster

Morphology:
    ask book is unusually sparse / structured

Flow:
    aggressive activity is elevated
```

Opportunity may use those facts to describe a richer typed hypothesis.

It MUST NOT convert them into a generic vote count.

---

## 18. Planner Consumption

Planner asks:

> **Given the current opportunity and context, is risking finite capital economically preferable to the alternatives?**

Perspectives may affect:

- which economic model is appropriate;
- uncertainty;
- expected adverse excursion;
- execution cost assumptions;
- expected time to resolution;
- conditional first-passage estimates;
- comparison with other candidates.

### Rule

> **No Perspective is a generic planner gate.**

Bad:

```text
if morphology_score < 0.7:
    reject
```

Better:

```text
morphology reports unusually thin, highly persistent structure
→ economic model uses the relevant execution/adverse-excursion context
→ Planner still evaluates the intervention
```

---

## 19. PositionRisk Consumption

This is a primary motivation for the Advisor layer.

The position should not lose the context that justified taking the risk.

At entry, Planner hands the PositionGuardian a compact `PositionContract`.

The contract may include:

```text
opportunity archetype
opportunity phase
admitted wallet risk budget
expected resolution horizon
entry execution regime
calibrated excursion information
calibrated passage information
model provenance
```

During the position, Advisors continue producing current Perspectives.

The guardian's risk model may use the latest available Perspectives to distinguish:

```text
ordinary adverse excursion inside a still-valid context
```

from:

```text
the context that justified entry has materially changed
```

### 19.1 Risk invariant

The Advisor layer MUST NOT allow Risk to violate the admitted loss budget.

If more price room is justified, quantity and loss budget must already make that room affordable.

The governing inequality remains conceptually:

\[
WorstCaseExecutableLoss \le AdmittedWalletRisk
\]

### 19.2 Hard floor semantics

The hard floor remains.

Its meaning becomes:

> **Regardless of every Advisor and every model, the admitted wallet-loss budget ends here.**

It is the catastrophe boundary.

It should not be the only intelligence managing the position.

---

## 20. Hindsight Consumption

Hindsight should replay the Perspective timeline around every opportunity and position.

It should be able to ask:

```text
What did HistoricalAnalogue say at entry?
What did Coordination say?
Was morphology already abnormal?
Did liquidity deteriorate before the stop?
Did the current path remain inside prior successful adverse-excursion envelopes?
Did the opportunity context survive the stop?
Did a relationship break before exit?
Did the position later recover?
```

This permits diagnoses such as:

```text
bad detection
late entry
bad valuation
correct stop
premature stop
bad management
missed re-entry
relationship decay
liquidity-regime failure
```

without simply blaming the final exit trigger.

---

# Part IV — Offline Universe Discovery

## 21. Why Discovery Is Offline

Live SYMM must remain sparse and streaming.

Research is different.

Offline research is explicitly allowed to perform exhaustive operations such as:

```text
all symbols
× all symbols
× rolling windows
× historical episodes
```

because it runs outside the live trading path.

For approximately 640 symbols:

```text
unordered pairs ≈ 204,480
directed pairs ≈ 408,960
```

That is acceptable for offline research.

It is not acceptable as blindly retained live state.

---

## 22. Existing Capture Infrastructure

The capture store already records raw venue transport payloads with arrival timestamps.

Replay already runs a capture through the full production stack.

Research SHOULD reuse this infrastructure so that:

- book reconstruction is identical;
- signal calculations are identical;
- event ordering is identical;
- signal baselines are identical;
- the research pass does not invent a second semantics for market data.

### Principle

> **Research may change how broadly we compare outputs. It should not silently change how production Measurements are defined.**

---

## 23. Research Run Provenance

Every research result MUST record:

```text
code commit
capture IDs
capture time range
symbol universe
signal specification versions
market depth configuration
analysis definitions
train/validation/test split
algorithm parameters
random seed where applicable
result version
```

A discovered pattern without provenance is not promotable into live use.

---

## 24. Causal Time Discipline

Research MUST distinguish:

```text
information available at time t
```

from:

```text
what happened after t
```

Live-equivalent features at time `t` may use only observations available by `t`.

Future observations may be used later as:

- outcome labels,
- episode classification,
- MFE/MAE,
- first-passage results,
- recurrence validation.

Future observations MUST NOT leak into the feature state that supposedly existed at `t`.

---

## 25. Discovery Pipeline

The recommended pipeline is:

```text
Capture
   ↓
production replay / feature extraction
   ↓
time-aligned Measurement/Category/Perspective primitives
   ↓
broad universe scan
   ↓
relationship episodes / motifs / outliers
   ↓
historical recurrence analysis
   ↓
future-outcome annotation
   ↓
held-out validation
   ↓
Research Catalog
   ↓
live Advisor configuration / reference models
```

---

## 26. Research Experiment A — Emergent Pair Relationships

### 26.1 Goal

Find pairs whose relationship becomes unusually strong or structured relative to their own history.

### 26.2 For each pair

Evaluate rolling state such as:

```text
contemporaneous correlation
best-lag correlation
best-lag seconds
correlation gain
lag prominence
lag curvature
pair SNR
correlation divergence
lag divergence
historical relationship-path percentile
```

### 26.3 Detect relationship episodes

A relationship episode begins when a pair moves from ordinary pair state into a statistically distinguishable local relationship.

The exact detection method MUST be derived from:

- pair historical baseline,
- pair residual noise,
- estimator support,
- or recurrence distance.

Do not use arbitrary universal thresholds where a causal pair baseline exists.

### 26.4 Episode record

Store:

```text
pair
orientation
start time
end time
duration
baseline state
peak/current correlation
lag path
correlation-gain path
prominence / curvature
support
SNR
historical recurrence
```

### 26.5 Cross-signal enrichment

For interesting episodes, compare:

```text
volume
Hawkes
liquidity
CVD
depthflow
morphology
derivatives
Category
```

The goal is to learn whether temporary coupling tends to be isolated to price or appears simultaneously in other measured dimensions.

---

## 27. Research Experiment B — Riding the Coattails

### 27.1 Goal

Find recurring temporary relationships in which one symbol repeatedly exhibits a measured temporal precedence over another during a coupling episode.

### 27.2 Requirements

A Coattail research candidate should preserve:

```text
relationship onset
pair baseline
best-lag path
correlation gain
lag localization
support
episode duration
recurrence count
```

### 27.3 Evidence for repeated precedence

Research may ask:

```text
When this coupling appears,
does X repeatedly precede Y?

Is the lag stable enough to localize?

Does lagged correlation improve materially over zero lag?

Does the relationship recur across independent episodes?

Does it survive held-out captures?

Is the structure still present net of broad market movement?
```

### 27.4 No causal promotion

Even a strong result remains:

```text
X has repeatedly preceded Y in these measured historical episodes.
```

Not:

```text
X causes Y.
```

And not:

```text
Y will follow X next time.
```

---

## 28. Research Experiment C — Dynamic Clusters

### 28.1 Goal

Discover temporary groups of symbols whose relationships strengthen together.

The cluster is time-local.

Do not assume permanent sectors.

### 28.2 Construction

Use validated pair episode edges to build a temporary relationship graph.

Possible edge quantities:

```text
current correlation divergence
best-lag relationship
relationship recurrence
shared morphology movement
shared flow movement
shared liquidity movement
```

### 28.3 Outputs

Research may identify:

```text
cluster members
cluster onset
cluster duration
internal dispersion
central / early-moving members
relationship orientations
cluster dissolution
```

A live Advisor may later describe current membership in a known/current coordination cluster.

---

## 29. Research Experiment D — Book Morphology Population Scan

### 29.1 Goal

Determine:

- what book shapes are common across the current universe;
- which symbols are outliers;
- whether morphology families recur;
- whether temporary morphology changes co-occur with price/flow/liquidity episodes.

### 29.2 Inputs

Use normalized morphology features:

```text
concentration
entropy
center of mass
dispersion
bid/ask JS divergence
spacing regularity
size quantization
persistence
turnover
replenishment cadence
cancellation cadence
```

### 29.3 Questions

Research may ask:

```text
Does a stable common morphology population exist?

Which symbols depart most strongly from it?

Do the same symbols repeatedly occupy extreme morphology states?

Do morphology changes precede or accompany known Category transitions?

Do coordinated symbols also show synchronized morphology changes?
```

No actor-intent labels are assigned.

---

## 30. Research Experiment E — Historical Motifs / Analogues

### 30.1 Goal

Discover recurring local trajectories in one symbol's own multivariate state.

### 30.2 State vector

Use standardized, dimensionally coherent features.

Examples:

```text
liquidity divergence
liquidity velocity
Hawkes SNR
flow divergence
book morphology
Category sequence
relative-state metrics
```

### 30.3 Methods

Matrix-profile / motif-discord methods are suitable where the trajectory can be represented as fixed or comparably supported subsequences.

Other distance methods are allowed when their semantics are explicit.

### 30.4 Output

For each current/historical episode:

```text
nearest motif
distance
distance percentile
support
alignment
duration
terminal observed category
MFE
MAE
time to resolution
```

Again, eventual outcome is annotation of history, not part of the earlier state.

---

## 31. Research Experiment F — Position Excursion Families

### 31.1 Goal

Give PositionRisk empirical context for how successful and failed positions actually traveled.

For each entered position or opportunity-aligned hypothetical entry:

```text
entry context
opportunity type
opportunity phase
Perspective state
MAE
MFE
time to MAE
time to MFE
profit first / invalidation first
time to resolution
eventual exit state
```

### 31.2 Use

This is the basis for a real conditional adverse-excursion model.

It should eventually replace generic execution-noise-only stop geometry where sufficient evidence exists.

---

# Part V — Research Validation

## 32. Discovery Is Not Validation

A relationship found by searching 200,000 pairs is expected to produce false discoveries.

A research result MUST NOT become live Advisor knowledge simply because it looked impressive in one capture.

---

## 33. Multiple Search Accounting

Where inferential statistics are used, research MUST account for the fact that many pairs/windows/lags were searched.

Possible tools include:

- family-wise correction,
- false discovery rate control,
- permutation/null experiments,
- held-out validation.

The appropriate method depends on the estimator and dependency structure.

No universal p-value threshold is mandated.

---

## 34. Time-Split Validation

Preferred validation is temporal.

Example:

```text
Discovery captures:
    A, B, C

Validation captures:
    D, E

Final untouched test:
    F
```

A relationship promoted from discovery should demonstrate that its descriptive behavior remains present out of sample.

---

## 35. Stability Questions

Before promotion, research should ask:

```text
Does the relationship recur?

Is its orientation stable?

Is the lag stable enough to be meaningful?

Is the relationship merely broad-market beta?

Does it remain after controlling for common market movement?

Does it depend on one extraordinary episode?

Does it survive different liquidity states?

Does it disappear when transaction cadence changes?

Is the sample count actually sufficient?
```

---

## 36. Research Catalog

Validated research output SHOULD be persisted as a versioned catalog.

Conceptually:

```text
Advisor model definitions
validated pair candidates
relationship baselines
historical motif references
morphology population baselines
excursion reference distributions
support/provenance metadata
```

The catalog is descriptive knowledge.

It is not a file of trading rules.

---

# Part VI — Live Sparse Promotion

## 37. Offline Exhaustive, Live Sparse

This distinction is mandatory:

```text
RESEARCH:
    exhaustive universe scan is allowed

LIVE:
    only admitted relationships maintain expensive resident pair state
```

Do not move the research cardinality into production.

---

## 38. Relationship Admission

Live Coordination/Coattail state may be created from:

1. validated relationships in the Research Catalog;
2. cheap current candidate discovery;
3. explicit economically related markets such as spot/perpetual;
4. current opportunity-related peers.

Detailed pair state is admitted only when justified.

---

## 39. Relationship Retirement

Live pair state MUST be retireable.

When a relationship:

- becomes stale,
- loses support,
- remains ordinary for a sufficiently evidenced period,
- or is no longer relevant,

its expensive resident state may be released.

Retirement semantics must be explicit and bounded.

---

## 40. No O(N²) Permanent Universe

The following is forbidden:

```text
640 symbols
× 639 peers
× history
× estimator state
× RLS state
```

merely because the universe contains those possible pairs.

Total live state should scale closer to:

```text
symbols
+
active opportunities
+
admitted relationships
+
open positions
+
bounded advisor state
```

---

# Part VII — Workspace and Performance

## 41. Streaming Invariant

> **Events move. State stays.**

Advisors MUST NOT introduce:

- snapshots,
- clones,
- global state batches,
- growing slices,
- hidden work queues,
- per-symbol mailboxes,
- custom transport rings,
- unbounded history.

---

## 42. LMAX

Existing Workspace transport remains LMAX/go-disruptor.

Advisors do not replace or wrap LMAX with custom general-purpose rings.

---

## 43. Service Classes

Suggested semantics:

```text
PriorityControl
    position protection / executions / safety

Realtime
    cheap opportunity and immediately relevant context

Analytics
    expensive advisor estimation / model fitting / research-like live work

UI
    observational latest-state presentation
```

Do not solve an expensive Advisor by promoting it to PriorityControl.

---

## 44. Degraded Advisor State

Advisors are context, not safety authority.

If an Advisor falls behind:

- latest-state Perspectives may coalesce;
- observational updates may drop according to explicit policy;
- diagnostics MUST show degraded context;
- PositionRisk MUST continue with its contract and hard safety boundary.

The UI should be able to display:

```text
ADVISORY CONTEXT DEGRADED

Coordination: stale
HistoricalAnalogue: current
Liquidity: current
Position protection: healthy
```

---

# Part VIII — Diagnostics

## 45. Advisor Diagnostics

Per Advisor, expose:

```text
input rate
output rate
pending
drops
coalesces
last update age
active symbol count
active relationship count
resident state count
Step latency
permit wait
```

Do not hide multiple subscriber rings behind one aggregate capacity.

---

## 46. Perspective Diagnostics

For a selected symbol show the current descriptive state.

Example:

```text
DENT/USD

HistoricalAnalogue
    nearest distance ...
    percentile ...
    match count ...
    stage ...

Coordination
    peer CAMP/USD
    current correlation ...
    best lag ...
    onset ...
    persistence ...

Morphology
    entropy percentile ...
    spacing regularity percentile ...
    quantization percentile ...

Liquidity
    ask depth divergence ...
    divergence velocity ...
```

No action recommendation belongs in this panel.

---

# Part IX — Tests and Acceptance

## 47. Advisor Unit Tests

Every Advisor needs tests for:

- causal ordering;
- missing input;
- stale input;
- bounded state;
- deterministic output;
- no fabricated zero for undefined data;
- no unbounded allocations after warmup.

---

## 48. Allocation Tests

Hot Advisor update paths SHOULD have allocation benchmarks.

Particularly:

```text
steady-state same-symbol update
steady-state admitted-pair update
Perspective emission
Perspective key comparison
```

Comparator and identity operations MUST allocate zero.

---

## 49. Streaming Tests

Prove:

- no global snapshot is created;
- no hidden event backlog exists;
- same key preserves FIFO where required;
- unrelated keys may progress independently;
- current state remains bounded.

---

## 50. Coordination Tests

Construct:

1. historically weak pair;
2. locally strengthening relationship;
3. stable non-zero lag;
4. later dissolution.

Expected:

```text
Perspective describes onset
Perspective preserves orientation
Perspective reports local relationship
Perspective later reports dissolution/staleness
```

No causality label is emitted.

---

## 51. Historical Analogue Tests

Construct several stored episodes and one current trajectory.

Expected:

```text
nearest match is correct
distance ordering is correct
current stage alignment is correct
future portion of current episode is never used
```

---

## 52. PositionRisk Integration Test

Construct:

```text
open position
hard risk budget
ordinary adverse excursion
current Perspective still resembles successful historical family
```

Expected:

- Perspective is available to PositionRisk;
- hard maximum wallet risk remains unchanged;
- Advisor cannot move the catastrophe boundary beyond admitted risk.

Then construct context collapse.

Expected:

- PositionRisk can respond before the hard floor if its own model justifies it;
- Planner does not need to own the position.

---

## 53. Research Reproducibility Test

Given:

```text
same capture
same code commit
same research configuration
```

the research pass must produce the same catalog/result set, excluding explicitly documented nondeterministic algorithms.

---

# Part X — Initial Implementation Plan

## 54. Phase 1 — Research Harness

Build an offline `research` command/module that:

1. selects one or more capture IDs;
2. replays/extracts production Measurements;
3. writes compact research feature streams;
4. performs broad universe analysis;
5. emits versioned research results.

Do not modify live Workspace scheduling for this work.

---

## 55. Phase 2 — First Two Advisors

Start with the two clearest use cases.

### 55.1 HistoricalAnalogueAdvisor

Deliver:

```text
nearest historical trajectory
distance
percentile
support
stage alignment
historical excursion summaries
```

### 55.2 CoordinationAdvisor

Deliver:

```text
current pair relationship
relationship divergence from baseline
lag
correlation gain
onset
persistence
historical recurrence
```

These directly test the ideas discussed around:

- DENT,
- CAMP,
- precursor history,
- and position tolerance.

---

## 56. Phase 3 — MorphologyAdvisor

Implement normalized order-book morphology and cross-sectional comparison.

This should produce measured structural facts, not "bot" classification.

---

## 57. Phase 4 — PositionRisk Consumption

Allow open positions to consume the latest relevant Perspectives without making the guardian depend on Analytics progress.

Keep:

```text
absolute wallet risk boundary
```

as the independent hard safety constraint.

---

## 58. Phase 5 — Opportunity / Planner Consumption

Use Perspectives to enrich:

- opportunity state,
- economic model selection,
- excursion assumptions,
- time-to-resolution assumptions,
- and arbitration.

Do not add generic Perspective gates.

---

## 59. Phase 6 — Hindsight Attribution

Record Perspective state at:

```text
candidate first seen
candidate armed
entry
peak
MAE
exit
post-exit recovery
```

Use it to attribute:

```text
detection
valuation
selection
risk
management
```

failures separately.

---

# Part XI — Non-Goals

This work does NOT authorize:

- a new universal scoring system;
- a new generic confidence scalar;
- hard-coded "bot" detection;
- hard-coded manipulation labels;
- direct Advisor trading authority;
- Planner ownership of open positions;
- replacing PositionGuardian;
- replacing LMAX;
- all-pairs live permanent state;
- hidden work queues;
- world snapshots;
- cloned histories;
- metric maps on hot paths;
- future leakage in research;
- fitting a relationship and validating it on the same observations without accounting for search.

---

# Part XII — Architectural Lock

The following wording is intended to be copied into agent instructions.

> **Advisor is an operational context producer. Perspective is its descriptive output.**

> **A Perspective describes the past and present; it does not guarantee the future.**

> **Perspectives are nuance, not gates.**

> **Signals remain the authoritative owners of their Measurements; Advisors compose existing outputs rather than re-deriving them.**

> **Use the inter-signal relationship sections in the signal specifications as the initial Advisor composition map.**

> **Opportunity, Planner, PositionRisk, and Hindsight may consume the same Perspective for different purposes. No Advisor owns their decisions.**

> **Planner decides whether to risk capital. PositionRisk owns an admitted position.**

> **The intelligence that justified a position must not disappear at handoff; relevant Perspective context remains available to PositionRisk.**

> **The hard floor is the absolute wallet-risk boundary, not the only source of position intelligence.**

> **Offline research may exhaustively scan the universe. Live SYMM must remain sparse.**

> **Research discovers candidate relationships; held-out validation earns promotion into live Advisor knowledge.**

> **Do not interpret temporary synchronization as proof of manipulation or causality. Describe exactly what was measured.**

> **Events move. State stays. No snapshots, clones, hidden accumulation, or permanent all-to-all live state.**

---

## 60. Target System

The intended end-to-end structure is:

```text
RAW MARKET
    ↓
SIGNALS
    ↓
MEASUREMENTS
    ├───────────────┬───────────────┬───────────────┐
    ↓               ↓               ↓               ↓
CATEGORY        MANIFOLD        RELATION        ADVISORS
    │               │               │               │
    │               │               │          PERSPECTIVES
    │               │               │               │
    └───────────────┴───────┬───────┴───────────────┘
                            ↓
                       OPPORTUNITY
                            ↓
                         PLANNER
                            ↓
                    POSITION CONTRACT
                            ↓
                    POSITION GUARDIAN
                            │
                    POSITION RISK MODEL
                            │
                  latest Perspectives
                            │
                            ↓
                         EXECUTION
                            ↓
                        HINDSIGHT

OFFLINE:
CAPTURES
    ↓
PRODUCTION REPLAY
    ↓
UNIVERSE DISCOVERY
    ↓
VALIDATION
    ↓
RESEARCH CATALOG
    ↓
LIVE ADVISOR REFERENCE STATE
```

The purpose is not to make SYMM omniscient.

The purpose is to let the system retain and use the enormous amount of context it already measures, without collapsing that context into brittle universal gates.

A symbol may be unusual for its own history.

A book may be unusual relative to the universe.

Two symbols may suddenly begin behaving together.

A current position may be experiencing an adverse excursion that was common in historically similar episodes.

A relationship may be dissolving.

A liquidity vacuum may be temporary or may be accelerating.

Those are all useful things to know.

They are Perspectives.

What to do about them remains somebody else's job.
