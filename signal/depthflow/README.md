# Depthflow Signal Specification

## 1. Purpose

The depthflow signal measures how displayed order-book depth is distributed between bid and ask and how that displayed depth is added, removed, and redistributed through time.

It answers four measurement questions:

1. How asymmetric is the displayed book?
2. Does the touch have the same asymmetry as the full observed book?
3. How much displayed notional is appearing and disappearing on each side?
4. How unusual and persistent are those changes relative to the market's own causal history?

The signal does not determine whether displayed depth is genuine, spoofed, supportive, resistant, bullish, bearish, loaded, thin, or manipulative.

---

## 2. First Principles

At event time \(t\), the observed book contains bid and ask levels:

\[
\mathcal{B}_t=\{(p_i^b,q_i^b)\}
\]

\[
\mathcal{A}_t=\{(p_j^a,q_j^a)\}
\]

For each level, displayed quote-currency notional is:

\[
n_i=p_iq_i
\]

The total displayed notional on each side is therefore:

\[
\boxed{
B_t=\sum_{i\in\mathcal{B}_t}p_i^bq_i^b
}
\]

\[
\boxed{
A_t=\sum_{j\in\mathcal{A}_t}p_j^aq_j^a
}
\]

The fundamental static directional measurement is book imbalance:

\[
\boxed{
I^{book}_t=
\frac{B_t-A_t}{B_t+A_t}
}
\]

when \(B_t+A_t>0\).

The touch provides a second resolution. Let:

\[
D^b_t=p^b_1q^b_1
\]

\[
D^a_t=p^a_1q^a_1
\]

Then:

\[
\boxed{
I^{touch}_t=
\frac{D^b_t-D^a_t}
{D^b_t+D^a_t}
}
\]

The difference between these two measurements is itself informative:

\[
\boxed{
G_t=I^{touch}_t-I^{book}_t
}
\]

A non-zero \(G_t\) means that near-touch asymmetry differs from whole-book asymmetry. The signal reports the difference and does not assign a cause.

Depthflow is fundamentally temporal. For a price level \(p\), let displayed notional change between two observations be:

\[
\Delta n_t(p)=n_t(p)-n_{t-1}(p)
\]

Positive changes are additions:

\[
\boxed{
Add_t=\sum_p\max(\Delta n_t(p),0)
}
\]

Negative changes are removals:

\[
\boxed{
Remove_t=\sum_p\max(-\Delta n_t(p),0)
}
\]

These quantities describe visible book change only.

A removal does not by itself identify whether liquidity was:

- executed;
- cancelled;
- repriced;
- replaced elsewhere;
- removed for another reason.

Attribution belongs to signals that observe the required trade and order-event evidence.

---

## 3. Inputs

### 3.1 Required book observations

For each level:

| Input           | Unit          | Validity             |
|-----------------|---------------|----------------------|
| price           | quote/base    | finite, positive     |
| quantity        | base quantity | finite, non-negative |
| side            | bid / ask     | explicit             |
| event timestamp | time          | monotonic per symbol |

The snapshot or delta stream MUST identify an observation domain consistently enough that changes in coverage are not mistaken for additions or removals.

### 3.2 Valid observation forms

The signal MAY consume either:

1. authoritative ordered-book deltas; or
2. complete, comparable snapshots.

Event deltas are preferred for additions and removals because they preserve the actual book mutation.

If snapshots are used, a flow measurement is valid only over price levels whose observation coverage is comparable between the two snapshots.

A change in feed depth, truncation, reconnect state, or snapshot coverage MUST NOT be reported as market flow.

### 3.3 Relationship to trades

Executed trades are not required input to depthflow.

Trade observations belong to execution-flow signals.

They may be combined downstream with depthflow measurements.

---

## 4. Measurement Envelope

Every measurement contains:

- `From`;
- `At`;
- `Maturity`;
- `SNR`.

### 4.1 `At`

`At` is the event time of the book state represented by the measurement.

### 4.2 `From`

For state-only metrics:

\[
From=At
\]

For flow metrics:

\[
From=t_{previous}
\]

where \(t_{previous}\) is the previous comparable book observation.

For baseline, momentum, SNR, or historical-path measurements, `From` is the earliest retained observation contributing non-zero weight to the joint estimator represented by the measurement.

### 4.3 `Maturity`

For weighted historical estimators:

\[
N_{\mathrm{eff}}
=
\frac{(\sum_iw_i)^2}
{\sum_iw_i^2}
\]

and:

\[
\boxed{
Maturity=
\begin{cases}
0,&N_{\mathrm{eff}}\le1\\[4pt]
1-\frac{1}{N_{\mathrm{eff}}},&N_{\mathrm{eff}}>1
\end{cases}
}
\]

The measurement-level maturity is the minimum maturity required by the joint SNR.

### 4.4 `SNR`

Define the core state vector:

\[
X_t=
\begin{bmatrix}
I^{book}_t\\
G_t\\
F^{signed}_t\\
C_t
\end{bmatrix}
\]

where:

- \(I^{book}\) is whole-book imbalance;
- \(G\) is touch/full-book resolution gap;
- \(F^{signed}\) is normalized signed net displayed flow rate;
- \(C\) is normalized book-turnover rate.

Let:

\[
\mu_{t-}
\]

be the causal mean state and:

\[
\Sigma_{t-}
\]

its causal residual covariance.

Then:

\[
\delta_t=X_t-\mu_{t-}
\]

and:

\[
\boxed{
SNR_t=
\frac{1}{4}
\delta_t^\top
\Sigma_{t-}^{-1}
\delta_t
}
\]

`SNR` is undefined until the covariance model is estimable and non-degenerate.

It is an unbounded non-negative ratio and is not a probability.

---

## 5. Static Book Metrics

### 5.1 `book_notional:bid`

\[
\boxed{
B_t=\sum_i p_i^bq_i^b
}
\]

**Unit:** quote currency.

**Meaning:** total displayed bid notional within the observation domain.

**Why:** direct aggregation of resting displayed value.

**Downstream use:** contextualize book flow, full-book contraction/expansion, and liquidity measurements.

**Required provenance:** observation-domain depth or full-book status.

---

### 5.2 `book_notional:ask`

\[
\boxed{
A_t=\sum_j p_j^aq_j^a
}
\]

Same contract as bid book notional.

---

### 5.3 `book_notional`

\[
\boxed{
T_t=B_t+A_t
}
\]

**Unit:** quote currency.

**Meaning:** total displayed notional within the observed book domain.

**Downstream use:** normalization of flow and book-turnover metrics.

---

### 5.4 `book_imbalance`

\[
\boxed{
I^{book}_t=
\frac{B_t-A_t}{B_t+A_t}
}
\]

**Range:** `[-1,1]`.

**Meaning:** signed asymmetry of total displayed notional.

- positive: more observed bid notional;
- negative: more observed ask notional;
- zero: equal observed notionals.

**Non-claim:** imbalance does not imply future direction or intent.

---

### 5.5 `touch_imbalance`

With touch notionals \(D^b_t,D^a_t\):

\[
\boxed{
I^{touch}_t=
\frac{D^b_t-D^a_t}
{D^b_t+D^a_t}
}
\]

**Range:** `[-1,1]`.

**Meaning:** signed asymmetry at the executable touch.

**Why it is included:** it provides the near-market resolution required to compare the touch with the full book.

**Relationship:** the absolute touch capacity itself belongs to the liquidity measurement domain.

---

### 5.6 `imbalance_resolution_gap`

\[
\boxed{
G_t=I^{touch}_t-I^{book}_t
}
\]

**Range:** `[-2,2]`.

**Meaning:** difference between touch and whole-book asymmetry.

Examples:

- \(G=0\): both resolutions have the same signed imbalance;
- \(G>0\): touch is more bid-heavy than the full book;
- \(G<0\): touch is more ask-heavy than the full book.

**Why:** preserves disagreement between resolutions without classifying it.

**Downstream use:** combine with cancellation, replenishment, book morphology, or execution flow to investigate why resolutions differ.

---

### 5.7 `imbalance_resolution_distance`

\[
\boxed{
D^{resolution}_t=
|I^{touch}_t-I^{book}_t|
}
\]

**Range:** `[0,2]`.

**Meaning:** unsigned magnitude of touch/full-book disagreement.

**Why:** useful where the magnitude of disagreement matters independently of its direction.

---

## 6. Displayed Flow Metrics

For side \(s\in\{bid,ask\}\), compare comparable book states at \(t_0\) and \(t_1\).

Let:

\[
\Delta t=t_1-t_0>0
\]

and:

\[
\Delta n^s(p)=n^s_{t_1}(p)-n^s_{t_0}(p)
\]

### 6.1 `added_notional:{bid,ask}`

\[
\boxed{
A^s=
\sum_p\max(\Delta n^s(p),0)
}
\]

**Unit:** quote currency.

**Meaning:** displayed notional that appeared on that side during the interval.

**Non-claim:** an addition does not identify the actor, motive, or persistence of the order.

---

### 6.2 `removed_notional:{bid,ask}`

\[
\boxed{
R^s=
\sum_p\max(-\Delta n^s(p),0)
}
\]

**Unit:** quote currency.

**Meaning:** displayed notional that disappeared from that side during the interval.

**Non-claim:** removal does not distinguish fill, cancellation, retreat, or repricing.

---

### 6.3 `net_displayed_flow:{bid,ask}`

\[
\boxed{
F^s=A^s-R^s
}
\]

Equivalently, under identical observation coverage:

\[
F^s=T^s_{t_1}-T^s_{t_0}
\]

**Unit:** quote currency.

**Meaning:** net change in displayed notional on the side.

---

### 6.4 `added_notional_rate:{bid,ask}`

\[
\boxed{
\dot A^s=\frac{A^s}{\Delta t}
}
\]

**Unit:** quote currency / second.

---

### 6.5 `removed_notional_rate:{bid,ask}`

\[
\boxed{
\dot R^s=\frac{R^s}{\Delta t}
}
\]

**Unit:** quote currency / second.

---

### 6.6 `net_displayed_flow_rate:{bid,ask}`

\[
\boxed{
\dot F^s=\frac{F^s}{\Delta t}
}
\]

**Unit:** quote currency / second.

**Why:** removes dependence on irregular book-update cadence.

---

## 7. Scale-Free Flow Metrics

Let reference depth over the interval be:

\[
\boxed{
T^{ref}=\frac{T_{t_0}+T_{t_1}}{2}
}
\]

when \(T^{ref}>0\).

### 7.1 `book_turnover_rate`

Define gross displayed mutation:

\[
M=
A^{bid}+R^{bid}+A^{ask}+R^{ask}
\]

Then:

\[
\boxed{
C_t=
\frac{M}
{T^{ref}\Delta t}
}
\]

**Unit:** \(1/\text{second}\).

**Meaning:** fraction of displayed book notional being replaced, added, or removed per unit time.

**Why:** distinguishes a stable book from a rapidly changing book without assigning a cause.

**Downstream use:** compare with Hawkes event intensity, cancellation rates, or morphology persistence.

---

### 7.2 `net_book_change_rate`

\[
\boxed{
N_t=
\frac{T_{t_1}-T_{t_0}}
{T^{ref}\Delta t}
}
\]

**Unit:** \(1/\text{second}\).

**Meaning:** scale-free rate at which total displayed book notional expands or contracts.

---

### 7.3 `signed_net_displayed_flow_rate`

Define directional net change:

\[
F^\Delta=F^{bid}-F^{ask}
\]

Then:

\[
\boxed{
F^{signed}_t=
\frac{F^{bid}-F^{ask}}
{T^{ref}\Delta t}
}
\]

**Unit:** \(1/\text{second}\).

**Meaning:** signed rate at which displayed depth is accumulating more strongly on one side than the other.

Positive values indicate relatively greater bid-side accumulation or ask-side depletion.

Negative values indicate relatively greater ask-side accumulation or bid-side depletion.

**Non-claim:** this is not buy/sell pressure and does not predict price direction.

---

### 7.4 `flow_activity_imbalance`

Let gross mutations by side be:

\[
M^b=A^b+R^b
\]

\[
M^a=A^a+R^a
\]

Then:

\[
\boxed{
I^{activity}_t=
\frac{M^b-M^a}
{M^b+M^a}
}
\]

when \(M^b+M^a>0\).

**Range:** `[-1,1]`.

**Meaning:** which side of the book is undergoing more visible change, irrespective of whether that change is addition or removal.

---

## 8. Historical Baselines

Historical baselines are maintained causally for at least:

\[
I^{book}
\]

\[
G
\]

\[
F^{signed}
\]

\[
C
\]

The current observation is evaluated against the pre-observation baseline before the estimator is updated.

### 8.1 `book_imbalance_baseline`

\[
\boxed{
\mu^{I}_{t-}=E[I^{book}]_{t-}
}
\]

### 8.2 `book_imbalance_divergence`

\[
\boxed{
d^I_t=I^{book}_t-\mu^I_{t-}
}
\]

### 8.3 `book_imbalance_zscore`

\[
\boxed{
z^I_t=
\frac{d^I_t}
{\sigma^I_{t-}}
}
\]

where \(\sigma^I\) is causal residual dispersion.

---

### 8.4 `resolution_gap_baseline`

\[
\boxed{
\mu^G_{t-}=E[G]_{t-}
}
\]

### 8.5 `resolution_gap_divergence`

\[
\boxed{
d^G_t=G_t-\mu^G_{t-}
}
\]

### 8.6 `resolution_gap_zscore`

\[
\boxed{
z^G_t=
\frac{d^G_t}{\sigma^G_{t-}}
}
\]

**Use:** measure whether touch/full-book disagreement is ordinary or unusual for this market.

It does not explain the disagreement.

---

### 8.7 `turnover_baseline`

Because \(C_t\ge0\), model positive turnover in log space when \(C_t>0\):

\[
y^C_t=\log C_t
\]

and retain the zero-turnover state explicitly.

For positive observations:

\[
\boxed{
B^C_t=e^{\mu^C_{t-}}
}
\]

### 8.8 `turnover_ratio`

\[
\boxed{
R^C_t=
\frac{C_t}{B^C_t}
}
\]

when both are positive.

### 8.9 `turnover_zscore`

\[
\boxed{
z^C_t=
\frac{\log C_t-\mu^C_{t-}}
{\sigma^C_{t-}}
}
\]

**Use:** distinguish ordinary book churn from unusually rapid displayed-book mutation.

---

## 9. Temporal Dynamics

### 9.1 `book_imbalance_velocity`

Fit a causal local regression:

\[
I^{book}_i=
\alpha+\beta_I(t_i-t)+\epsilon_i
\]

Then:

\[
\boxed{
v_I=\beta_I
}
\]

**Unit:** imbalance / second.

**Meaning:** temporal movement of full-book asymmetry.

---

### 9.2 `resolution_gap_velocity`

\[
G_i=
\alpha+\beta_G(t_i-t)+\epsilon_i
\]

\[
\boxed{
v_G=\beta_G
}
\]

**Unit:** gap / second.

**Meaning:** whether touch/full-book disagreement is widening, narrowing, or stable.

---

### 9.3 `turnover_velocity`

When enough positive turnover observations exist:

\[
\log C_i=
\alpha+\beta_C(t_i-t)+\epsilon_i
\]

\[
\boxed{
v_C=\beta_C
}
\]

**Unit:** log-turnover / second.

**Meaning:** change in the market's displayed-book mutation rate.

---

### 9.4 Slope quality

For each fitted slope:

\[
\boxed{
SNR_\beta=
\frac{\beta^2}
{\operatorname{Var}(\beta)}
}
\]

MAY be emitted when enough support exists.

This distinguishes persistent movement from regression noise.

---

## 10. Historical Recurrence

The signal MAY retain a standardized trajectory:

\[
Z_t=
\begin{bmatrix}
z^I_t\\
z^G_t\\
z^{F}_t\\
z^C_t
\end{bmatrix}
\]

where \(z^F\) is the standardized signed displayed-flow rate.

The current trajectory is compared with non-overlapping historical trajectories over the same causally derived support.

Recommended metrics:

### 10.1 `historical_path_distance`

Distance to the nearest historical depthflow trajectory.

### 10.2 `historical_path_percentile`

Empirical percentile of that nearest-match distance within retained history.

### 10.3 `historical_match_from`

Start time of the nearest prior trajectory.

These measurements describe recurrence or novelty only.

No regime label is emitted.

---

## 11. Proximity Weighting

A distance-weighted depth imbalance MAY be added only when its weighting kernel is explicitly specified and justified.

For example, a generic form is:

\[
I_w=
\frac{
\sum_i w(r_i)n_i^b-\sum_j w(r_j)n_j^a
}{
\sum_i w(r_i)n_i^b+\sum_j w(r_j)n_j^a
}
\]

where \(r\) is a dimensionless distance from the touch or midpoint.

The signal specification MUST state:

- the exact definition of \(r\);
- the exact kernel \(w(r)\);
- why that kernel is appropriate;
- which parameter, if any, determines its scale.

A weighting kernel MUST NOT be introduced merely because nearer depth feels more important.

The unweighted full-book and touch measurements remain the required core.

---

## 12. Relationship to Liquidity

Liquidity measures executable touch capacity and execution-price geometry.

Depthflow measures the distribution and temporal redistribution of displayed depth.

Useful downstream combinations include:

### 12.1 Touch contraction versus full-book redistribution

Liquidity may show falling touch depth while depthflow shows stable total book notional.

That combination means displayed depth moved away from the touch or was redistributed elsewhere in the observed book.

If both touch depth and total book notional fall, the contraction is broader.

The signals report these facts independently.

### 12.2 Spread versus book redistribution

A widening spread combined with high book turnover differs from a widening spread under an otherwise static book.

No causal conclusion is assigned.

### 12.3 Morphology

Dimensionless book-shape metrics from liquidity can be combined with:

- `book_turnover_rate`;
- `resolution_gap`;
- `resolution_gap_velocity`;
- `signed_net_displayed_flow_rate`.

This relates book shape to its temporal mutation.

---

## 13. Relationship to Toxicity / Order Attribution

Depthflow measures that displayed quantity appeared or disappeared.

A toxicity/order-accounting signal may identify whether disappearance was associated with:

- executions;
- unchanged-price cancellations;
- touch retreat;
- replacement.

Examples of useful downstream combinations:

- high `removed_notional:bid` + high bid cancellation fraction;
- high `removed_notional:bid` + high bid fill fraction;
- high removal + rapid replacement;
- high `resolution_gap` + repeated touch cancellation.

Depthflow MUST NOT modify book imbalance by a toxicity score.

The observed book state and the attribution of its changes remain separate evidence.

---

## 14. Relationship to CVD / Executed Flow

Let executed aggressive buy and sell notional rates be:

\[
E_b,\quad E_s
\]

Depthflow supplies displayed-book change rates:

\[
\dot F^{bid},\quad \dot F^{ask}
\]

Useful downstream comparisons include:

\[
\frac{E_b}{D_a}
\]

\[
\frac{E_s}{D_b}
\]

from liquidity, together with:

\[
\dot F^{ask}
\]

or:

\[
\dot F^{bid}
\]

from depthflow.

Examples:

- aggressive buying while ask depth replenishes;
- aggressive buying while ask depth is removed;
- aggressive selling while bid depth replenishes;
- aggressive selling while bid depth contracts.

These are different observable configurations.

Depthflow does not label them absorption, resistance, support, or exhaustion.

---

## 15. Relationship to Hawkes / Event Intensity

Book mutation rate can be compared with event-arrival intensity.

Useful ratios include:

\[
\frac{\lambda_{\text{trades}}}{C}
\]

or separately comparing trade-arrival intensity with:

- added-notional rate;
- removed-notional rate;
- book-turnover rate.

A high event rate in a slowly changing book differs mechanically from a high event rate in a rapidly reconstructing book.

---

## 16. Relationship to Price Measurements

Price-return or midpoint-response measurements may be combined with:

- `book_imbalance`;
- `book_imbalance_divergence`;
- `book_imbalance_velocity`;
- `signed_net_displayed_flow_rate`;
- `book_turnover_rate`;
- `resolution_gap`.

The signal does not infer that book imbalance caused the return.

Downstream analysis may test that relationship.

---

## 17. Cross-Symbol Comparison

Absolute `book_notional`, addition, and removal quantities MUST NOT be compared across arbitrary symbols as if they shared a common economic scale.

The following dimensionless or normalized metrics MAY be compared across symbols when observation domains are compatible:

- `book_imbalance`;
- `touch_imbalance`;
- `imbalance_resolution_gap`;
- `imbalance_resolution_distance`;
- `flow_activity_imbalance`;
- `book_turnover_rate`, with event-time units preserved;
- `net_book_change_rate`;
- standardized historical divergences;
- standardized historical-path distances.

Cross-symbol clustering or classification remains downstream.

---

## 18. Invalid and Missing States

The signal MUST distinguish:

1. zero observed quantity;
2. missing book side;
3. invalid crossed/locked state when the venue does not define it as valid;
4. changed observation coverage;
5. absent previous comparable observation;
6. unavailable historical baseline;
7. unavailable noise covariance;
8. numerical estimator failure.

Rules:

- no previous comparable observation means flow metrics are undefined;
- unchanged book state produces zero flow, not missing flow;
- changed feed coverage produces undefined flow, not artificial additions/removals;
- zero gross mutation makes `flow_activity_imbalance` undefined;
- unavailable covariance makes SNR undefined.

---

## 19. Explicit Non-Claims

The depthflow signal does not determine:

- whether a book is spoofed;
- whether displayed depth is genuine;
- whether an imbalance is loaded;
- whether the market is thin;
- whether an order intends to trade;
- whether depth is support or resistance;
- whether book flow is bullish or bearish;
- whether trade flow confirms the book;
- whether a book mutation is toxic;
- whether a recurring trajectory will produce the same future outcome.

Those are downstream reasoning tasks.

---

## 20. Minimal Required Metric Set

A valid depthflow implementation SHOULD minimally publish:

- `book_notional:bid`;
- `book_notional:ask`;
- `book_notional`;
- `book_imbalance`;
- `touch_imbalance`;
- `imbalance_resolution_gap`;
- `imbalance_resolution_distance`;
- `added_notional:bid`;
- `added_notional:ask`;
- `removed_notional:bid`;
- `removed_notional:ask`;
- `net_displayed_flow:bid`;
- `net_displayed_flow:ask`;
- `added_notional_rate:bid`;
- `added_notional_rate:ask`;
- `removed_notional_rate:bid`;
- `removed_notional_rate:ask`;
- `net_displayed_flow_rate:bid`;
- `net_displayed_flow_rate:ask`;
- `book_turnover_rate`;
- `net_book_change_rate`;
- `signed_net_displayed_flow_rate`;
- `flow_activity_imbalance`;
- `book_imbalance_baseline`;
- `book_imbalance_divergence`;
- `book_imbalance_zscore`;
- `resolution_gap_baseline`;
- `resolution_gap_divergence`;
- `resolution_gap_zscore`;
- `turnover_baseline`;
- `turnover_ratio`;
- `turnover_zscore`;
- `book_imbalance_velocity`;
- `resolution_gap_velocity`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `Maturity`;
- `SNR`.
