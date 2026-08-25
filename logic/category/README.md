# Category / Market Regime Specification

## 1. Purpose

The category solver converts heterogeneous signal measurements into a discrete, interpretable description of the **current market regime for one symbol**.

Signals answer questions such as:

- how imbalanced is executed flow?
- how thin is displayed liquidity?
- how unusual is return energy?
- how strongly are arrivals self-exciting?
- how much is the order book changing?
- how synchronized is the symbol with its peers?
- how far is the present observation from its own historical baseline?

Category answers the downstream question:

> **What market state do those measurements jointly support?**

Examples include:

- `aggressive_drive`;
- `hidden_absorption`;
- `book_thinning`;
- `coiled_compression`;
- `vertical_ignition`;
- `turbulent`;
- `liquidity_vacuum`;
- `systemic_herd`;
- `active_reversal`;
- `equilibrium`.

A category is therefore a **regime hypothesis**, not a measurement.

The solver consumes `Measurement` objects from all signal modules, maintains a causal evidence state independently for each symbol, evaluates the declared category vocabulary, and publishes a ranked category distribution whose first element is the dominant regime.

That dominant regime is the discrete token consumed by `logic/cognition`.

The category solver MUST NOT:

- predict the next regime;
- use prior regime transitions as evidence for the current regime;
- consult Cognition's predictions;
- infer trading action;
- interpret publication frequency as evidence;
- treat missing measurements as zero;
- let asynchronous signal arrival order create artificial regime transitions;
- accumulate historical regime evidence indefinitely.

Its responsibility ends at:

\[
\boxed{
\text{current measurements}
\longrightarrow
\text{current regime distribution}
}
\]

Cognition owns:

\[
\boxed{
\text{regime sequence}
\longrightarrow
\text{transition model}
\longrightarrow
\text{next-regime prediction}
}
\]

---

## 2. Architectural Boundary

The analytical pipeline has three distinct semantic layers.

### 2.1 Signals measure

A signal publishes quantities with physical, statistical, or structural meaning.

For example:

\[
z_{\mathrm{flow}}
\]

may describe standardized executed-flow imbalance, while:

\[
\rho(K)
\]

may describe the spectral radius of a fitted Hawkes branching matrix.

Neither quantity says what the market **is**.

### 2.2 Category interprets

Category combines those measurements into hypotheses such as:

\[
\texttt{aggressive\_drive}
\]

or:

\[
\texttt{turbulent}
\]

This is the first stage allowed to attach semantic market-state labels to measurements.

### 2.3 Cognition predicts transitions

Cognition treats the dominant category as one symbol in a discrete language:

\[
c_1 \rightarrow c_2 \rightarrow c_3 \rightarrow \cdots
\]

and estimates:

\[
P(c_{t+1}\mid c_t,c_{t-1},\ldots)
\]

from the regime-transition history stored in its radix trie.

Category MUST therefore remain observational.

If Cognition's predicted next category were fed back into Category's present classification, the system would become circular:

\[
\text{past categories}
\rightarrow
\text{prediction}
\rightarrow
\text{current category}
\rightarrow
\text{training data}
\]

and Cognition would increasingly learn its own expectations.

That feedback is forbidden.

---

## 3. First Principles

A market regime is not directly observable.

It is a latent interpretation supported by multiple observable quantities.

Let the complete set of eligible signal measurements for symbol \(s\) at evaluation time \(t\) be:

\[
X_s(t)
=
\{
x_1,x_2,\ldots,x_n
\}
\]

Each \(x_i\) retains:

- signal source;
- metric identity;
- raw value;
- normalized or standardized representation when available;
- observation interval;
- event time;
- maturity;
- signal-to-noise information;
- peer identity when applicable.

For every declared category \(c\), the category schema selects measurements relevant to that hypothesis:

\[
E_c(s,t)
\subseteq
X_s(t)
\]

The solver maps those measurements into category evidence and derives a category strength:

\[
S_c(s,t)
\]

The complete category competition is:

\[
\mathcal{C}
=
\{c_1,c_2,\ldots,c_K\}
\]

and produces evidence-share confidences:

\[
P_c(s,t)
\]

with one dominant category:

\[
\boxed{
c^*(s,t)
=
\arg\max_{c\in\mathcal C}
P_c(s,t)
}
\]

The dominant category is the current regime token.

The important distinction is:

\[
\boxed{
S_c
\neq
P_c
}
\]

`Strength` describes how strongly the measurements support a category in its own evidence space.

`Confidence` describes that category's standing relative to the competing category hypotheses.

---

## 4. Per-Symbol Independence

Category state is always keyed by symbol.

For symbols \(A\) and \(B\):

\[
X_A(t)
\]

and:

\[
X_B(t)
\]

are independent evidence states.

A measurement belonging to \(A\) MUST NOT alter the category state of \(B\) unless the measurement itself explicitly describes \(B\) and is emitted with \(B\) as its symbol.

Cross-symbol measurements MAY contribute to a symbol's regime.

For example, a correlation measurement may describe \(A\) relative to peer \(B\).

In that case:

- `Symbol=A` determines which category state receives the evidence;
- `Peer=B` remains provenance;
- the same measurement MUST NOT silently classify \(B\).

If both orientations are required, the measuring signal must emit both explicitly or provide a declared cross-sectional aggregate.

---

## 5. Inputs

The category solver consumes `Measurement` objects from `ChannelMeasurements`.

For classification purposes the relevant measurement fields are:

| Field | Meaning |
|---|---|
| `Source` | signal that produced the evidence |
| `Symbol` | symbol whose regime may be affected |
| `Tick` | causal evaluation epoch when supplied |
| `At` | instant at which the measurement is valid |
| `ObservedFrom` | beginning of its observation interval |
| `Peer` | explicit peer identity for bivariate measurements |
| `PeerAt` | peer-side event time when applicable |
| `Maturity` | estimator support |
| `SNR` / `SNRDefined` | measurement departure relative to its own noise model |
| `Metrics` | named measured quantities |
| `Metadata` | additional explicit provenance |

Measurements with:

- missing symbol;
- invalid event time;
- a terminal measurement error;
- future timestamps relative to the category evaluation time;

MUST NOT enter the current evidence snapshot.

---

## 6. Category Evaluation Epoch

Category MUST classify a coherent state of the market rather than the incidental order in which asynchronous signals reached the worker pool.

For symbol \(s\), define a category evaluation epoch:

\[
T=(s,\tau)
\]

where \(\tau\) is normally the current engine tick or another explicit causal watermark.

For that epoch the solver constructs one evidence snapshot:

\[
X_s(\tau)
\]

containing the latest eligible measurement for every declared evidence coordinate as of \(\tau\).

The solver MUST publish at most one dominant category verdict for a symbol for one completed evaluation epoch.

Therefore this sequence:

```text
CVD arrives
DepthFlow arrives
Hawkes arrives
Liquidity arrives
```

MUST NOT inherently create:

```text
aggressive_drive
→ book_thinning
→ turbulent
→ liquidity_vacuum
```

merely because the four measurements were processed in that order.

Those measurements belong to one evidence state when they describe the same regime epoch.

The regime verdict must be invariant to their scheduling order.

Formally, for any permutation \(\pi\) of the same causal measurement set:

\[
\boxed{
C(X)
=
C(\pi(X))
}
\]

where \(C\) is category classification.

This invariant is essential because Cognition treats category changes as meaningful market transitions.

---

## 7. Causal Snapshot Construction

At evaluation time \(t\), an evidence item is eligible only when:

\[
At_i \le t
\]

The category solver MUST NOT use later measurements to revise an already-emitted live verdict.

For each evidence coordinate \(k\), select:

\[
\boxed{
x_k(t)
=
\operatorname*{arg\,max}_{x_i}
At_i
}
\]

subject to:

\[
At_i\le t
\]

and the coordinate identity matching \(k\).

This is **latest-state replacement**, not historical accumulation.

When a new measurement for the same coordinate arrives, it supersedes the older measurement for future category epochs.

The older value MUST NOT remain as another independent vote.

---

## 8. Evidence Coordinate Identity

A category evidence coordinate is identified by the information required to distinguish one analytical observation from another.

At minimum:

\[
k=
(Source,Metric)
\]

When identity depends on additional dimensions, the coordinate MUST preserve them.

Examples include:

\[
(Source,Metric,Side)
\]

or:

\[
(Source,Metric,Peer)
\]

The category solver MUST NOT collapse genuinely different observations merely because their metric strings happen to match.

Conversely, repeated publication of the same logical coordinate MUST NOT create additional independent evidence.

---

## 9. Category Schema

`types.CategorySchemas` is the declarative bridge between measurements and regime hypotheses.

A schema entry answers:

> Which measured phenomenon provides evidence about which market regime?

The minimum identity is:

```text
signal source
+ metric identity
→ category
```

For example:

```text
CVD:signed_net_fraction_zscore
→ aggressive_drive
```

The schema is interpretive configuration.

Signal implementations MUST NOT know which category consumes their measurements.

Category implementations MUST NOT reconstruct signal mathematics that already belongs to the signal.

The schema MAY eventually carry additional explicit evidence semantics such as:

- evidence polarity;
- support role;
- peer aggregation policy;
- freshness budget;
- evidence transform;
- weight;
- required versus corroborating status.

Those semantics MUST be explicit if introduced.

They MUST NOT be inferred from category names.

---

## 10. Evidence Transformation

Signal metrics do not all inhabit the same mathematical scale.

A raw:

\[
z\text{-score}
\]

cannot automatically be compared numerically with:

\[
\rho(K)
\]

a spread ratio, a correlation, or a notional rate.

Therefore a category input must have a defined evidence interpretation.

Let:

\[
x_i
\]

be the selected signal metric.

Its category affinity is:

\[
\boxed{
a_i=\phi_i(x_i)
}
\]

where \(\phi_i\) is the declared evidence transform.

For the current schema, a metric's `Normalized` value MAY be used directly when that normalized quantity already has the documented semantics required by the category classifier.

Category MUST NOT assume that every field named `Normalized` is automatically comparable merely because it exists.

If a signal exposes only a physically meaningful raw or standardized quantity, Category owns the transformation from that quantity into hypothesis evidence.

Signals MUST NOT distort their own output into an arbitrary `[0,1]` classifier score merely to satisfy Category.

This preserves the architectural rule:

> Signals measure truthfully; Category decides what the measurements imply.

---

## 11. Positive Evidence

For category \(c\), let its currently present supporting affinity values be:

\[
A_c=
\{a_1,a_2,\ldots,a_n\}
\]

with:

\[
a_i>0
\]

A positive value means that the corresponding measurement provides positive evidence for the category under the declared schema.

A genuine measured zero means:

\[
a_i=0
\]

and provides no positive support.

A negative metric value MUST NOT automatically be converted into positive evidence for the opposite semantic category.

For example:

```text
not aggressive_drive
```

does not automatically imply:

```text
hidden_absorption
```

unless an explicit schema says that the measured phenomenon supports that hypothesis.

---

## 12. Missing Evidence

The following states remain distinct:

1. measured positive support;
2. measured zero support;
3. measured opposition;
4. measurement not present;
5. measurement stale;
6. metric not estimable;
7. invalid measurement.

Missing evidence is not zero evidence.

If a schema leg has no eligible current measurement, it belongs in:

```text
Missing
```

and MUST NOT be inserted into the strength calculation as a fabricated zero.

This prevents an unavailable signal from becoming evidence against a category.

---

## 13. Publication Frequency Must Not Be Evidence

Suppose one CVD metric is published 100 times while one DepthFlow metric is published once during the same regime interval.

The 100 CVD publications do not constitute 100 independent arguments.

For each declared evidence coordinate, Category retains the latest eligible state.

Therefore:

\[
\boxed{
\text{one coordinate}
=
\text{one current vote}
}
\]

A signal's publication cadence MUST NOT increase its category weight.

This is especially important when different signal modules naturally operate on:

- quote events;
- trades;
- Level 3 updates;
- volume bars;
- model refits;
- slower cross-sectional windows.

Without coordinate replacement, the fastest signal would dominate the regime classifier regardless of informational content.

---

## 14. Cross-Peer Evidence

Pairwise signals require additional care.

If a symbol is correlated against 200 peers, 200 pairwise measurements MUST NOT automatically count as 200 independent arguments for a systemic regime.

That would make category confidence depend on universe size.

Where the intended concept is cross-sectional, Category SHOULD consume a signal-supplied cohort statistic such as:

```text
cohort_signed_correlation
```

or use an explicit peer aggregation defined by the category schema.

Peer aggregation MUST be invariant to duplicated peers and MUST document how dependence among peer measurements is handled.

The category classifier MUST NOT pretend pair observations are conditionally independent merely because they occupy different queue records.

---

## 15. Category Strength

A category may receive corroborating evidence from several distinct measured phenomena.

The current combination rule is a geometric evidence mean.

For positive supporting affinities:

\[
A_c=
\{a_1,\ldots,a_n\}
\]

define:

\[
\boxed{
S_c
=
\left(
\prod_{i=1}^{n}a_i
\right)^{1/n}
}
\]

or equivalently:

\[
\boxed{
\log S_c
=
\frac{1}{n}
\sum_{i=1}^{n}
\log a_i
}
\]

when all participating values are positive.

If no positive supporting evidence exists:

\[
\boxed{
S_c=0
}
\]

The geometric mean is appropriate because corroborating evidence behaves conjunctively:

- one enormous measurement cannot additively overwhelm every other leg;
- repeated values do not scale strength linearly;
- a weak supporting leg reduces the combined strength;
- evidence remains on the same multiplicative scale.

If explicit schema weights \(w_i\) are introduced, the extension is:

\[
\boxed{
S_c
=
\exp
\left(
\frac{
\sum_iw_i\log a_i
}{
\sum_iw_i
}
\right)
}
\]

with:

\[
w_i>0
\]

Weights MUST have declared semantics.

They MUST NOT be introduced merely to tune historical trading profitability.

---

## 16. Supporting Evidence Provenance

Every emitted category MUST expose the exact observations that argued for it.

A supporting identity SHOULD be representable as:

```text
source:metric
```

and include peer identity when needed.

Examples:

```text
cvd:signed_net_fraction_zscore
depthflow:net_book_change_rate
hawkes:branching_spectral_radius
correlation:cohort_signed_correlation
```

`Supporting` is provenance, not explanation prose.

A consumer must be able to trace a category verdict back to the measurements from which it was formed.

Duplicate supporting identities MUST be removed.

---

## 17. Opposing Evidence

Opposition is allowed only when explicitly defined.

Let:

\[
O_c
\]

be the measurements whose semantics directly contradict category \(c\).

Opposing evidence MUST NOT be manufactured by simply negating every supporting metric.

For example, the absence of high excitation is not automatically evidence for `laminar`.

A measurement belongs in `Opposing` only when the category schema explicitly declares the contradiction relationship.

The initial category implementation MAY operate solely with positive supporting evidence.

If so:

```text
Opposing = []
```

is preferable to fabricated opposition.

---

## 18. Maturity

Category maturity describes the weakest estimator support among the measurements actually supporting the regime.

For supporting measurements:

\[
m_1,m_2,\ldots,m_n
\]

define:

\[
\boxed{
M_c
=
\min_i m_i
}
\]

with:

\[
0\le M_c\le1
\]

The conservative minimum is used because a conjunctive regime claim is only as mature as the least mature measurement materially contributing to it.

If no supporting measurement reports maturity, Category MUST distinguish that condition from a measured maturity of zero.

Maturity does not affect category identity unless explicitly specified.

It remains estimator-quality provenance.

---

## 19. Freshness

Regime classification describes **now**.

Old measurements cannot remain current evidence indefinitely.

For each schema coordinate, the implementation MUST define a defensible freshness policy based on:

- the signal's observation timescale;
- event cadence;
- estimator horizon;
- or an explicit schema freshness budget.

An item with:

\[
At_i > t
\]

is future data and forbidden.

An item older than its permitted freshness horizon is stale and becomes missing for the current category evaluation.

If a normalized freshness value is reported, one valid representation for a declared maximum age \(T_i\) is:

\[
\boxed{
f_i
=
\max
\left(
0,
1-\frac{t-At_i}{T_i}
\right)
}
\]

for:

\[
T_i>0
\]

A category-level conservative freshness is:

\[
\boxed{
F_c=\min_i f_i
}
\]

over the measurements supporting that category.

Freshness is provenance.

The classifier MUST NOT silently multiply category strength by freshness unless such weighting is explicitly part of the category model.

---

## 20. Category Competition

Let the category vocabulary be:

\[
\mathcal C=
\{c_1,\ldots,c_K\}
\]

and let each category have strength:

\[
S_1,\ldots,S_K
\]

The classifier compares **all declared categories**, not only categories with positive current evidence.

Using the existing symmetric one-pseudocount evidence-share model, the confidence of category \(c_i\) is conceptually:

\[
\boxed{
P_i
=
\frac{
S_i+\alpha
}{
\sum_{j=1}^{K}S_j+K\alpha
}
}
\]

with the symmetric prior:

\[
\alpha=1
\]

when that is the classifier's configured pseudocount.

The important semantic contract is:

- every declared category participates in the competition;
- no category receives a privileged prior unless explicitly declared;
- category confidence is relative to the complete vocabulary;
- zero-strength categories remain represented by the symmetric prior.

Because the evidence strengths are not necessarily calibrated statistical likelihoods, `Confidence` MUST be described as **evidence-share confidence** unless a stronger probabilistic calibration has been demonstrated.

It MUST NOT be presented as a guaranteed real-world probability that the regime is true.

---

## 21. Dominant Regime

The dominant regime is:

\[
\boxed{
c^*
=
\arg\max_c P_c
}
\]

Since the symmetric prior is equal for every category, this is equivalent to selecting the strongest supported category when positive evidence exists.

The first category in the published batch MUST be the dominant regime.

Cognition relies on that contract.

Ties MUST be deterministic.

The canonical tie-break order is `types.CategoryOrder`.

A tie MUST NOT be resolved by:

- map iteration order;
- goroutine scheduling;
- signal arrival order;
- allocation address;
- random choice.

---

## 22. Confidence

For category \(c\):

```text
Confidence
```

is its evidence-share confidence:

\[
\boxed{
Confidence_c=P_c
}
\]

It answers:

> Relative to the complete declared regime vocabulary, how much of the current category evidence belongs to this hypothesis?

It does not answer:

- how large the underlying measurements are;
- how mature their estimators are;
- how long the regime has persisted;
- how likely the regime is to continue;
- whether entering a trade is profitable.

Those are separate questions.

---

## 23. Strength

For category \(c\):

```text
Strength
```

is:

\[
\boxed{
Strength_c=S_c
}
\]

It answers:

> How strongly do the currently supporting measurements agree with this category, before category competition?

Two categories can therefore have similar strength but different confidence depending on the rest of the regime distribution.

Likewise a category may have moderate strength but high confidence when competing hypotheses have little evidence.

Confidence and Strength MUST remain separate.

---

## 24. Classification Surprisal

For a category with evidence-share confidence:

\[
P_c>0
\]

define:

\[
\boxed{
I_c
=
-\log_2 P_c
}
\]

and publish:

```text
Surprisal = I_c
```

**Unit:** bits.

This is **classification surprisal**.

It means:

> How surprising is this category inside the current category competition?

It is not Cognition's transition surprisal.

Cognition computes a different quantity:

\[
\boxed{
I_{\mathrm{transition}}
=
-\log_2
P(
c_t
\mid
c_{t-1},c_{t-2},\ldots
)
}
\]

The two quantities MUST NOT be conflated.

Category surprisal is cross-sectional across competing current explanations.

Cognition surprisal is temporal across learned regime transitions.

---

## 25. Distribution Uncertainty

The complete regime distribution contains information that winner confidence alone cannot express.

For:

\[
P_1,\ldots,P_K
\]

define Shannon entropy:

\[
\boxed{
H
=
-\sum_{i=1}^{K}
P_i\log_2 P_i
}
\]

The maximum entropy is:

\[
H_{\max}=\log_2K
\]

A normalized category ambiguity measure is therefore:

\[
\boxed{
U
=
\frac{H}{\log_2K}
}
\]

with:

\[
0\le U\le1
\]

Interpretation:

- low \(U\): evidence concentrates on a small number of regimes;
- high \(U\): the measurements do not clearly distinguish competing regimes.

If the existing `Category.Uncertainty` field carries this quantity, every category in one batch MUST report the same distribution-level value.

It MUST NOT be confused with:

```text
1 - Confidence
```

which is only the complement of one candidate's evidence share.

---

## 26. Output Contract

Category publishes:

```text
[]types.Category
```

on:

```text
ChannelCategories
```

keyed by symbol.

Every batch belongs to one symbol and one category evaluation epoch.

The batch MUST satisfy:

1. all entries have the same `Symbol`;
2. all entries have the same evaluation `At`;
3. entry `0` is the dominant category;
4. alternatives follow in deterministic descending confidence order;
5. equal-confidence alternatives use `CategoryOrder` as the tie-break;
6. each category appears at most once;
7. `Supporting` contains no duplicate evidence coordinate;
8. missing and stale evidence are not listed as support.

The batch SHOULD preserve all positively supported alternatives rather than only the winner.

The winner is required for Cognition.

The alternatives are valuable to:

- graph reasoning;
- diagnostics;
- ambiguity analysis;
- future calibration;
- human inspection.

---

## 27. The `none` State

`CategoryTypeNone` means:

> No defensible current market-regime verdict is available.

It does not mean:

- equilibrium;
- neutral;
- stochastic;
- low volatility;
- no opportunity;
- no position;
- no market activity.

Those are real market hypotheses and require positive evidence.

`none` represents classification absence.

Typical causes include:

- no eligible category evidence;
- all relevant measurements are stale;
- no schema-selected metric is currently estimable.

By default, `none` SHOULD NOT become an ordinary Cognition regime token.

Otherwise the radix trie learns transitions in and out of telemetry missingness rather than transitions in market state.

If Cognition is ever intended to model an explicit unknown state, that decision must be made deliberately in the Cognition specification.

---

## 28. Regime Persistence

Category does not decide whether a regime transition is significant by looking at its previous verdict.

Given the same current evidence:

\[
X_s(t)
\]

Category must return the same classification regardless of:

\[
c_{t-1}
\]

Therefore:

\[
\boxed{
P(c_t\mid X_t,c_{t-1})
=
P(c_t\mid X_t)
}
\]

inside Category.

Temporal persistence, sequence probability, and transition expectations belong to Cognition.

This separation prevents hysteresis added by Category from contaminating the transition statistics Cognition is explicitly designed to learn.

If output stabilization is required for operational reasons, it must be based on measurement/evaluation coherence rather than prior category identity.

---

## 29. Cognition Contract

For each symbol, Cognition consumes the dominant regime:

\[
c^*_t
\]

as one discrete token.

Category therefore provides the vocabulary of the market language.

For example:

```text
aggressive_drive
→ book_thinning
→ frenzy
→ exhaustion
```

becomes one learned regime sequence.

Category has a responsibility to make those tokens semantically meaningful.

A token MUST represent a market-state hypothesis, not:

- one queue event;
- one signal publication;
- one arbitrary threshold crossing;
- one scheduler phase;
- one transient partial evidence snapshot.

Category should therefore change only when the **joint measurement state** supports a different regime.

Cognition may then safely interpret:

\[
c_a\rightarrow c_b
\]

as a market-regime transition rather than an implementation artifact.

---

## 30. Vocabulary Stability

Category names are persistent analytical identities.

Once Cognition has trained on:

```text
aggressive_drive
```

renaming that category to:

```text
aggressive_flow
```

creates a different token.

The radix trie cannot know that the two names are semantically equivalent.

Therefore category vocabulary changes are data-schema migrations.

The following are breaking changes for Cognition memory:

- renaming a category;
- merging two categories;
- splitting one category;
- changing a category's semantic meaning while retaining its name.

Such changes MUST:

1. increment an explicit category schema/vocabulary version;
2. reset Cognition's incompatible learned state; or
3. provide an explicit migration.

`CategoryOrder` is also a stable serialization contract where numeric category indexes are used.

Existing indexes SHOULD remain stable.

New categories SHOULD be appended unless a versioned migration explicitly changes the order.

---

## 31. Regime Design Requirements

A category should describe a market condition sufficiently coherent to be useful as a temporal token.

A proposed new category SHOULD answer all of the following:

1. What market phenomenon does the regime claim?
2. Which independent measurements support it?
3. Which signal owns each measurement?
4. Why do those measurements jointly imply the regime?
5. What would make the regime unavailable?
6. What measurements explicitly oppose it, if any?
7. Is it distinct from an existing category?
8. Would a sequence transition into or out of this state be meaningful to Cognition?

Categories SHOULD prefer mechanistic interpretation over colorful naming.

The vocabulary should not become a duplicate catalog of every metric signals emit.

---

## 32. Cross-Signal Composition

The purpose of Category is to make joint interpretation possible without corrupting the measurement layer.

### 32.1 Aggressive drive

Executed-flow measurements may provide evidence that trading pressure is strongly one-sided.

Category may interpret such evidence as:

```text
aggressive_drive
```

The CVD signal itself must continue to report executed-flow quantities rather than the semantic label.

### 32.2 Hidden absorption

Large executed notional combined with unusually weak midpoint response can support the interpretation:

```text
hidden_absorption
```

The regime comes from the relationship between flow and price response.

Neither measurement alone needs to claim that absorption exists.

### 32.3 Vertical ignition

A corroborated ignition state may combine evidence from distinct mechanisms such as:

- abnormal volume/notional flow;
- clustered arrival excitation;
- thinning displayed liquidity;
- aggressive directional execution.

The value of the regime comes from combining independent views of the same event.

### 32.4 Systemic herd

Cross-symbol dependence metrics may support:

```text
systemic_herd
```

but the classification MUST use a defensible cohort or peer aggregation rather than treating every peer pair as an independent vote.

---

## 33. Correlated Evidence

Measurements are not automatically independent merely because they come from separate schema rows.

For example:

- two CVD metrics may derive from the same trade stream;
- two DepthFlow metrics may derive from the same order-book mutation;
- a raw divergence and its z-score share the same underlying observation.

Category MUST NOT describe its evidence-share confidence as a Bayesian posterior based on independent likelihood factors unless those independence assumptions have actually been justified.

The geometric mean limits additive duplication but does not make correlated evidence independent.

Where materially redundant schema legs exist, the schema SHOULD:

- choose the most informative representation;
- explicitly group them;
- or otherwise prevent one underlying observation from receiving several accidental votes.

---

## 34. Historical Memory

Category may retain the latest evidence required to form the current causal snapshot.

It MUST NOT retain an ever-growing historical list of category evidence.

Specifically, it must not implement:

```text
every positive measurement ever seen
→ append forever
→ geometric mean forever
```

because that would make the current regime increasingly reflect the entire process lifetime.

Historical state belongs in systems built to model history:

- signal estimators retain their own causal baselines;
- Cognition retains regime sequences;
- other learning systems retain their explicitly defined statistics.

Category retains current evidence.

A bounded implementation can therefore store approximately:

\[
O(
\text{symbols}
\times
\text{schema coordinates}
)
\]

rather than:

\[
O(
\text{measurements since boot}
)
\]

---

## 35. Determinism

Given:

- identical category schema;
- identical category vocabulary version;
- identical per-symbol measurement set;
- identical evaluation time;

Category MUST produce identical output.

Results MUST be independent of:

- goroutine schedule;
- workspace shard;
- map iteration;
- subscriber order;
- message arrival permutation within an epoch.

This applies to:

- winner identity;
- strength;
- confidence;
- maturity;
- supporting provenance;
- alternative ordering.

Deterministic replay is a required property.

---

## 36. Diagnostics

The category solver SHOULD expose enough state to explain a verdict without reproducing the entire classifier internally.

Useful diagnostics per symbol include:

- current evaluation epoch;
- dominant category;
- category strength;
- category confidence;
- category uncertainty;
- number of eligible evidence coordinates;
- number of stale coordinates;
- number of missing coordinates;
- oldest supporting evidence age;
- newest supporting evidence age;
- classification duration;
- output change count.

For each candidate, inspection SHOULD expose:

```text
category
strength
confidence
maturity
freshness
supporting
opposing
missing
```

The diagnostics surface should make this question answerable:

> Why is BTC/USD currently classified as `aggressive_drive` rather than `hidden_absorption`?

without requiring a debugger.

---

## 37. Failure Semantics

A category classification failure is not equivalent to `none`.

The following remain distinct:

### No evidence

A valid classification pass found no usable regime evidence.

Result:

```text
none
```

### Partial evidence

Some signals are unavailable but sufficient current evidence exists.

Result:

```text
normal ranked category batch
+ Missing provenance
```

### Invalid classifier input

A mathematical or schema invariant is violated.

Result:

```text
error
```

### Solver failure

A systemic category-processing error occurs.

Result:

```text
error propagated through workspace failure handling
```

An implementation error MUST NOT silently become a neutral-looking market regime.

---

## 38. Required Invariants

The implementation MUST preserve all of the following.

### 38.1 Causality

\[
At_{\mathrm{evidence}}
\le
At_{\mathrm{category}}
\]

for every supporting measurement.

### 38.2 Per-symbol isolation

Evidence for one symbol cannot mutate another symbol's state.

### 38.3 One current value per coordinate

Repeated publication replaces current evidence rather than increasing vote count.

### 38.4 Arrival-order invariance

Permuting asynchronous arrivals within one evaluation epoch does not change the verdict.

### 38.5 Missing is not zero

Unavailable evidence cannot become negative evidence by default.

### 38.6 Stable winner ordering

The dominant category is always element zero.

### 38.7 Stable tie breaking

Equal evidence resolves according to canonical vocabulary order.

### 38.8 No temporal feedback

Prior category identity and Cognition predictions do not affect current Category classification.

### 38.9 Bounded state

Memory usage is bounded by current per-symbol evidence requirements.

### 38.10 Vocabulary stability

A category token retains one semantic meaning for one vocabulary version.

---

## 39. Required Tests

The category module MUST include tests covering at least the following contracts.

### 39.1 Single evidence source

One eligible metric supporting one category produces the expected category strength and dominant verdict.

### 39.2 Multiple corroborating sources

Distinct measurements supporting the same category combine using the declared geometric evidence rule.

### 39.3 Weak-leg behavior

A weak corroborating measurement reduces a conjunctive geometric strength appropriately.

### 39.4 Duplicate publication

Publishing the same evidence coordinate repeatedly does not increase category strength.

### 39.5 Replacement

A newer value for one evidence coordinate replaces the older value.

### 39.6 Arrival-order permutation

Every permutation of one epoch's measurements produces identical output.

### 39.7 Per-symbol isolation

Interleaved measurements for two symbols produce the same results as processing each symbol separately.

### 39.8 Missing metric

A missing schema metric is recorded as missing and does not become zero evidence.

### 39.9 Stale metric

Expired evidence stops contributing to classification.

### 39.10 Future measurement

A measurement whose `At` exceeds the evaluation time cannot influence the current verdict.

### 39.11 No evidence

A symbol with no eligible regime evidence produces the explicit `none` state or no-regime artifact according to the channel contract.

### 39.12 Deterministic tie

Equal-strength categories resolve according to `CategoryOrder`.

### 39.13 Confidence consistency

The full category distribution obeys the classifier's normalization contract.

### 39.14 Surprisal consistency

For every emitted category:

\[
Surprisal
=
-\log_2(Confidence)
\]

within numerical tolerance.

### 39.15 Maturity provenance

Category maturity equals the declared conservative aggregation of its supporting measurements.

### 39.16 Peer-count invariance

Duplicating a cross-symbol peer measurement cannot artificially increase systemic-regime confidence.

### 39.17 Bounded memory

Long-running repeated updates of one coordinate do not grow category state without bound.

### 39.18 Cognition-facing stability

Several signal arrivals belonging to one market epoch produce one dominant category token rather than a sequence of partial-state tokens.

---

## 40. Non-Claims

A Category verdict does not claim:

- that the regime will persist;
- that the next regime is known;
- that the category is causal;
- that the dominant category is ground truth;
- that the category implies a buy or sell;
- that high confidence implies profitability;
- that low uncertainty implies predictive power;
- that supporting measurements are statistically independent;
- that category surprisal is transition surprisal;
- that the regime vocabulary is exhaustive.

Category is an interpretable classification of the current measured state.

Prediction begins downstream.

---

## 41. Relationship to Cognition

The complete boundary is:

\[
\boxed{
\text{market events}
\rightarrow
\text{signals}
\rightarrow
\text{measurements}
\rightarrow
\text{category}
\rightarrow
\text{regime token}
\rightarrow
\text{cognition}
}
\]

Signals preserve continuous evidence.

Category performs semantic compression:

\[
\mathbb{R}^n
\rightarrow
\mathcal C
\]

Cognition then learns structure in the resulting symbolic trajectory:

\[
\mathcal C^*
\rightarrow
P(c_{t+1}\mid\text{prefix})
\]

This compression is intentionally lossy.

The category vocabulary should preserve the distinctions that matter for transition reasoning while discarding measurement-level detail that Cognition does not need.

That makes Category one of the most important semantic boundaries in the system.

Bad measurements produce bad evidence.

Bad category semantics produce a bad language.

And a bad language produces a radix trie that learns transitions perfectly between states that never meant anything.

---

## 42. Files

| File | Responsibility |
|---|---|
| `logic/category/solver.go` | Per-symbol evidence state, epoch classification, evidence combination, category publication |
| `logic/category/README.md` | This specification |
| `types/category.go` | Category vocabulary, stable ordering, category schema, output type |
| `nomagique/types/measurement.go` | Signal-to-logic measurement boundary |
| `logic/cognition/solver.go` | Downstream regime tokenization and transition learning |
| `logic/cognition/README.md` | Cognition sequence-model contract |

---

## 43. Governing Principle

The category layer should satisfy one test above all others:

> If the order in which signal goroutines happened to finish changes the regime sequence learned by Cognition, Category is wrong.

The category solver exists to turn a causally coherent set of measurements into a causally coherent market-state token.

Everything else follows from that.