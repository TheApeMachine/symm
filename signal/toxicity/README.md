# Toxicity Signal Specification

## 1. Purpose

The toxicity signal accounts for what happened to previously displayed touch liquidity between two comparable book observations.

It measures:

1. how much of the previously displayed touch quantity was observably executed;
2. how much unfilled quantity disappeared because the touch price moved away;
3. how much quantity was withdrawn or replenished at an unchanged touch price beyond what observed fills explain;
4. how these quantities compare with the previous displayed amount;
5. how their rates and proportions differ from the market's own causal history;
6. whether the current attribution pattern is familiar or unusual relative to prior patterns.

The signal measures observable liquidity disposition. It does not infer order intent, deception, spoofing, sincerity, toxicity of a participant, or whether a withdrawal is benign or manipulative.

---

## 2. First Principles

For one side of the touch, let the previous observation at time \(t_0\) be:

\[
(P_0,Q_0)
\]

and the current observation at time \(t_1>t_0\) be:

\[
(P_1,Q_1)
\]

where \(P\) is the best price and \(Q\) is displayed quantity.

Let:

\[
\Delta t=t_1-t_0
\]

The interval being attributed is:

\[
\boxed{(t_0,t_1]}
\]

Trades in that interval are used only when they can be causally matched to the previously displayed touch.

For the bid, resting bid liquidity is executed by aggressive sells at the previous bid price.

For the ask, resting ask liquidity is executed by aggressive buys at the previous ask price.

Let raw matched trade quantity be \(E^\ast\). The maximum quantity that can be attributed to the previous displayed touch is the quantity that was actually displayed:

\[
\boxed{
E=\min(E^\ast,Q_0)
}
\]

The expected unfilled residual of the previous touch is therefore:

\[
\boxed{
U=\max(Q_0-E,0)
}
\]

All further accounting begins from \(U\).

### 2.1 Unchanged touch price

If:

\[
P_1=P_0
\]

then the previous price is still the touch.

The current quantity \(Q_1\) is compared with the expected residual \(U\).

Net unexplained withdrawal is:

\[
\boxed{
W=\max(U-Q_1,0)
}
\]

Net replenishment is:

\[
\boxed{
A=\max(Q_1-U,0)
}
\]

These are **net** quantities.

If cancellations and additions both occurred at the same price between observations, two snapshots cannot reconstruct their gross amounts unless order-level events are available.

Therefore the signal MUST NOT describe \(W\) as gross cancellation or \(A\) as gross addition when only snapshot-plus-trade accounting is available.

### 2.2 Touch retreats away from the market

For the bid, retreat is:

\[
P_1<P_0
\]

For the ask, retreat is:

\[
P_1>P_0
\]

Under a valid ordered book, a less aggressive new best price implies that the previous touch price no longer contains displayed liquidity.

The unfilled residual that disappeared with that price-level retreat is:

\[
\boxed{
R=U
}
\]

This is a structural attribution: the prior touch level disappeared while the best price moved away.

It does not identify why the orders at that level disappeared.

### 2.3 Touch improves toward the market

For the bid, improvement is:

\[
P_1>P_0
\]

For the ask, improvement is:

\[
P_1<P_0
\]

The previous touch may still exist deeper in the book.

With touch-only observations, the disposition of \(U\) is therefore undefined.

If a complete book observation provides the current quantity at the previous price \(P_0\), the unchanged-level accounting MAY be applied to that old price level.

The signal MUST NOT assume that an improved touch means the previous touch was cancelled, retained, or replaced.

---

## 3. Inputs

### 3.1 Required book observations

For each side:

| Input | Unit | Validity |
|---|---|---|
| previous best price | quote/base | finite, positive |
| previous touch quantity | base quantity | finite, non-negative |
| current best price | quote/base | finite, positive |
| current touch quantity | base quantity | finite, non-negative |
| previous timestamp | time | required |
| current timestamp | time | strictly later than previous |

The previous and current observations MUST belong to the same venue, instrument, and book stream.

### 3.2 Required trade observations for fill attribution

For every trade in \((t_0,t_1]\):

| Input | Unit | Validity |
|---|---|---|
| trade price | quote/base | finite, positive |
| trade quantity | base quantity | finite, positive |
| aggressor side | buy / sell | explicit |
| event timestamp | time | inside attribution interval |

Fill attribution requires a complete and ordered trade interval.

If feed continuity is broken or cannot satisfy the adapter's completeness contract, fill-dependent attribution is undefined.

### 3.3 Optional order-level observations

If order IDs or authoritative per-order deltas are available, the signal MAY additionally measure gross cancellation, gross addition, and order survival directly.

Such metrics MUST be explicitly distinguished from net snapshot-derived accounting.

---

## 4. Measurement Envelope

Every measurement contains:

- `From`;
- `At`;
- `Maturity`;
- `SNR`.

### 4.1 `From`

For an attributed bracket:

\[
\boxed{From=t_0}
\]

For a point touch observation without a previous comparable touch:

\[
From=At
\]

For historical trajectory metrics, `From` extends to the earliest retained observation contributing non-zero weight to the represented estimator.

### 4.2 `At`

\[
\boxed{At=t_1}
\]

The measurement is valid as of the current book observation.

### 4.3 `Maturity`

For historical estimators with weights \(w_i\):

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

Direct attribution metrics do not require statistical maturity.

When the measurement contains baseline-dependent metrics and joint SNR, measurement-level `Maturity` is the minimum maturity required by that joint estimator.

### 4.4 `SNR`

Define the attribution state from the available side fractions:

\[
X_t=
\begin{bmatrix}
f^b_t\\
f^a_t\\
w^b_t\\
w^a_t\\
r^b_t\\
r^a_t\\
a^b_t\\
a^a_t
\end{bmatrix}
\]

where:

- \(f\) = fill fraction;
- \(w\) = same-price net withdrawal fraction;
- \(r\) = retreat fraction;
- \(a\) = same-price net replenishment fraction.

Only components that are defined for the current bracket participate.

Let the corresponding causal mean and residual covariance be:

\[
\mu_{t-},\quad \Sigma_{t-}
\]

For the \(k\) defined components:

\[
\delta_t=X_t-\mu_{t-}
\]

and:

\[
\boxed{
SNR_t=
\frac{1}{k}
\delta_t^\top
\Sigma_{t-}^{-1}
\delta_t
}
\]

`SNR` is undefined until the relevant causal covariance is estimable and non-degenerate.

SNR measures how unusual the joint liquidity-disposition pattern is relative to its historical noise.

It does not measure confidence that the activity was manipulative or toxic.

---

## 5. Touch Provenance Metrics

### 5.1 `previous_best_price:{bid,ask}`

**Meaning:** touch price whose displayed quantity is being attributed.

**Unit:** quote/base.

**Downstream use:** audit fill matching and touch-price movement.

---

### 5.2 `best_price:{bid,ask}`

**Meaning:** current best bid or ask at `At`.

**Unit:** quote/base.

---

### 5.3 `previous_touch_quantity:{bid,ask}`

\[
\boxed{Q_0}
\]

**Unit:** base quantity.

**Meaning:** maximum displayed quantity that can be attributed from the previous touch.

---

### 5.4 `touch_quantity:{bid,ask}`

\[
\boxed{Q_1}
\]

**Unit:** base quantity.

**Meaning:** current quantity at the current touch.

---

### 5.5 `touch_price_log_change:{bid,ask}`

\[
\boxed{
\Delta p=\log(P_1/P_0)
}
\]

**Unit:** dimensionless log ratio.

**Why:** price-scale-independent description of touch movement.

**Downstream use:** distinguish unchanged, retreating, and improving touch geometry.

---

## 6. Trade and Fill Metrics

### 6.1 `bracket_trade_quantity`

\[
\boxed{
V=\sum_{j:t_0<t_j\le t_1}q_j
}
\]

**Unit:** base quantity.

**Meaning:** total executed quantity observed during the attribution interval.

**Why:** provenance for how active the tape was during the bracket.

**Non-claim:** not all bracket trades interacted with the previous touch.

---

### 6.2 `matched_touch_trade_quantity:{bid,ask}`

For bid:

\[
\boxed{
E^{\ast,b}
=
\sum
q_j
\quad
\text{for sell-aggressor trades with }p_j=P^b_0
}
\]

For ask:

\[
\boxed{
E^{\ast,a}
=
\sum
q_j
\quad
\text{for buy-aggressor trades with }p_j=P^a_0
}
\]

**Unit:** base quantity.

**Meaning:** trade-tape quantity consistent with executing at the previous touch price.

This raw matched quantity is preserved separately from the physically attributable fill.

---

### 6.3 `touch_fill_quantity:{bid,ask}`

\[
\boxed{
E=\min(E^\ast,Q_0)
}
\]

**Unit:** base quantity.

**Meaning:** quantity observably executable against the previously displayed touch, capped by its displayed quantity.

**Why:** attribution cannot exceed the amount previously observed.

---

### 6.4 `touch_fill_fraction:{bid,ask}`

For \(Q_0>0\):

\[
\boxed{
f=\frac{E}{Q_0}
}
\]

**Range:** `[0,1]`.

**Meaning:** fraction of previously displayed touch quantity accounted for by observed executions.

**Downstream use:** compare execution against disappearance, replenishment, liquidity depth, and price response.

**Non-claim:** a high or low fill fraction does not establish sincerity or intent.

---

### 6.5 `touch_fill_rate:{bid,ask}`

\[
\boxed{
\dot E=\frac{E}{\Delta t}
}
\]

**Unit:** base quantity / second.

**Why:** removes irregular bracket-duration effects.

---

## 7. Residual Accounting Metrics

### 7.1 `unfilled_residual_quantity:{bid,ask}`

\[
\boxed{
U=\max(Q_0-E,0)
}
\]

**Unit:** base quantity.

**Meaning:** previous touch quantity not explained by matched fills.

This is the accounting base for withdrawal, replenishment, or retreat attribution.

---

## 8. Same-Price Withdrawal Metrics

Defined when the relevant current price equals the previous attributed price.

### 8.1 `net_withdrawn_quantity:{bid,ask}`

\[
\boxed{
W=\max(U-Q_1,0)
}
\]

**Unit:** base quantity.

**Meaning:** net disappearance at an unchanged price beyond what observed fills explain.

**Why:** under snapshot accounting:

\[
Q_1=U-C+A
\]

where \(C\) is gross cancellation/withdrawal and \(A\) is gross addition.

Therefore:

\[
W=\max(C-A,0)
\]

The signal observes only the positive net balance unless order-level events are available.

---

### 8.2 `net_withdrawal_fraction:{bid,ask}`

\[
\boxed{
w=\frac{W}{Q_0}
}
\]

for \(Q_0>0\).

**Range:** `[0,1]`.

**Downstream use:** compare unexplained touch withdrawal with depthflow, liquidity, and execution flow.

---

### 8.3 `net_withdrawal_rate:{bid,ask}`

\[
\boxed{
\dot W=\frac{W}{\Delta t}
}
\]

**Unit:** base quantity / second.

---

## 9. Same-Price Replenishment Metrics

Defined when the relevant current price equals the previous attributed price.

### 9.1 `net_replenished_quantity:{bid,ask}`

\[
\boxed{
A=\max(Q_1-U,0)
}
\]

**Unit:** base quantity.

**Meaning:** net quantity appearing at the same price beyond the residual expected after observed fills.

Under snapshot accounting:

\[
A=\max(A_{gross}-C_{gross},0)
\]

It is not gross addition.

---

### 9.2 `net_replenishment_fraction:{bid,ask}`

\[
\boxed{
a=\frac{A}{Q_0}
}
\]

for \(Q_0>0\).

**Range:** `[0,\infty)`.

The fraction may exceed one because newly displayed quantity may exceed the previous touch quantity.

---

### 9.3 `net_replenishment_rate:{bid,ask}`

\[
\boxed{
\dot A=\frac{A}{\Delta t}
}
\]

**Unit:** base quantity / second.

---

## 10. Retreat Metrics

A retreat is defined geometrically.

Bid retreat:

\[
P^b_1<P^b_0
\]

Ask retreat:

\[
P^a_1>P^a_0
\]

### 10.1 `retreated_quantity:{bid,ask}`

\[
\boxed{
R=U
}
\]

when the touch moves away and the previous price level is no longer present by book ordering.

**Unit:** base quantity.

**Meaning:** unfilled previous touch quantity that disappeared as the executable price moved away.

**Non-claim:** this does not state why that quantity disappeared.

---

### 10.2 `retreat_fraction:{bid,ask}`

\[
\boxed{
r=\frac{R}{Q_0}
}
\]

for \(Q_0>0\).

**Range:** `[0,1]`.

---

### 10.3 `retreat_rate:{bid,ask}`

\[
\boxed{
\dot R=\frac{R}{\Delta t}
}
\]

**Unit:** base quantity / second.

---

## 11. Touch Improvement and Unresolved Disposition

When the touch improves toward the market, touch-only data cannot determine what happened to the previous touch level.

### 11.1 `previous_level_disposition`

The dependent withdrawal, replenishment, and retreat metrics are **undefined** unless the current book still exposes the previous price level.

If full-book data provides quantity \(Q_1(P_0)\), the same-price residual equations MAY be applied to the previous price level:

\[
W(P_0)=\max(U-Q_1(P_0),0)
\]

\[
A(P_0)=\max(Q_1(P_0)-U,0)
\]

The signal MUST preserve whether the attribution came from:

- touch-only bracketing; or
- full-book previous-level observation.

---

## 12. Optional Order-Level Metrics

When authoritative order IDs or per-order deltas are available, the signal MAY publish:

- `gross_cancelled_quantity:{bid,ask}`;
- `gross_added_quantity:{bid,ask}`;
- `order_survival_fraction:{bid,ask}`;
- `cancelled_order_count:{bid,ask}`;
- `added_order_count:{bid,ask}`.

These MUST NOT be synthesized from aggregate snapshots.

For order-level data:

\[
\boxed{
gross\_cancelled\_quantity
=
\sum_{\text{cancel events}}q
}
\]

\[
\boxed{
gross\_added\_quantity
=
\sum_{\text{add events}}q
}
\]

The provenance MUST identify that these are event-derived rather than net snapshot-derived quantities.

---

## 13. Historical Baselines

The signal maintains causal historical baselines for the attribution fractions:

\[
f,\quad w,\quad r,\quad a
\]

and for their event-time rates where useful.

Fractions are naturally additive bounded variables and are modeled directly.

The current observation is evaluated against the pre-observation estimator before the estimator is updated.

### 13.1 `fill_fraction_baseline:{bid,ask}`

\[
\boxed{
\mu^f_{t-}=E[f]_{t-}
}
\]

### 13.2 `fill_fraction_divergence:{bid,ask}`

\[
\boxed{
d^f_t=f_t-\mu^f_{t-}
}
\]

### 13.3 `fill_fraction_zscore:{bid,ask}`

\[
\boxed{
z^f_t=
\frac{d^f_t}{\sigma^f_{t-}}
}
\]

---

### 13.4 `withdrawal_fraction_baseline:{bid,ask}`

\[
\boxed{
\mu^w_{t-}=E[w]_{t-}
}
\]

### 13.5 `withdrawal_fraction_divergence:{bid,ask}`

\[
\boxed{
d^w_t=w_t-\mu^w_{t-}
}
\]

### 13.6 `withdrawal_fraction_zscore:{bid,ask}`

\[
\boxed{
z^w_t=
\frac{d^w_t}{\sigma^w_{t-}}
}
\]

---

### 13.7 `retreat_fraction_baseline:{bid,ask}`

\[
\boxed{
\mu^r_{t-}=E[r]_{t-}
}
\]

### 13.8 `retreat_fraction_zscore:{bid,ask}`

\[
\boxed{
z^r_t=
\frac{r_t-\mu^r_{t-}}
{\sigma^r_{t-}}
}
\]

---

### 13.9 `replenishment_fraction_baseline:{bid,ask}`

Because replenishment fraction is non-negative and unbounded, positive observations SHOULD be modeled in log space:

\[
y^a_t=\log a_t
\]

with zero replenishment retained as an explicit zero state.

For \(a_t>0\):

\[
\boxed{
z^a_t=
\frac{\log a_t-\mu^a_{t-}}
{\sigma^a_{t-}}
}
\]

---

## 14. Temporal Dynamics

The rate at which attribution fractions change can be useful independently of their current level.

For any supported fraction \(x_t\), fit a causal local regression:

\[
x_i=\alpha+\beta(t_i-t)+\epsilon_i
\]

Then:

\[
\boxed{
v_x=\beta
}
\]

Recommended metrics:

- `fill_fraction_velocity:{bid,ask}`;
- `withdrawal_fraction_velocity:{bid,ask}`;
- `retreat_fraction_velocity:{bid,ask}`;
- `replenishment_fraction_velocity:{bid,ask}`.

**Unit:** fraction / second, except log-replenishment velocity when modeled in log space.

When enough regression support exists:

\[
\boxed{
SNR_\beta=
\frac{\beta^2}
{\operatorname{Var}(\beta)}
}
\]

MAY be emitted as slope-specific signal-to-noise.

Acceleration is not required.

---

## 15. Historical Recurrence

The signal MAY retain a standardized attribution trajectory:

\[
Z_t=
\begin{bmatrix}
z^{f,b}_t\\
z^{f,a}_t\\
z^{w,b}_t\\
z^{w,a}_t\\
z^{r,b}_t\\
z^{r,a}_t\\
z^{a,b}_t\\
z^{a,a}_t
\end{bmatrix}
\]

using only dimensions defined over the compared intervals.

The current trajectory is compared with non-overlapping historical trajectories over equivalent causal support.

Recommended metrics:

### 15.1 `historical_path_distance`

Distance to the closest prior attribution trajectory.

### 15.2 `historical_path_percentile`

Empirical percentile of the nearest-match distance within retained history.

### 15.3 `historical_match_from`

Start time of the nearest prior trajectory.

These metrics measure recurrence and novelty.

They do not classify a regime or infer participant behavior.

---

## 16. Relationship to Liquidity

Liquidity supplies the absolute displayed capacity against which disposition occurs.

Useful downstream combinations include:

### 16.1 Withdrawal relative to notional capacity

For bid notional \(D_b=P_bQ_b\), quantity-based withdrawal can be converted to notional using the attributed touch price:

\[
W^{notional}=P_0W
\]

and compared with the previous touch notional.

A large withdrawal fraction under large absolute depth and the same fraction under very small absolute depth are distinct economic states.

### 16.2 Replenishment after execution

High fill fraction together with high replenishment fraction means observed execution was followed by restored displayed quantity at the same price.

The signals do not label this absorption or support.

### 16.3 Retreat and spread

Touch retreat may be combined with liquidity's spread change.

A retreat that widens the spread differs mechanically from a retreat immediately replaced by another close touch.

---

## 17. Relationship to Depthflow

Depthflow measures aggregate displayed additions, removals, redistribution, and imbalance.

Toxicity measures touch-level disposition against the trade tape.

Useful downstream combinations include:

- high depthflow removal + high net touch withdrawal;
- high depthflow removal + high touch fill fraction;
- high depthflow turnover + low net touch withdrawal;
- widening touch/full-book imbalance gap + repeated touch retreat;
- stable total book notional + high touch withdrawal, indicating redistribution away from the attributed touch.

Neither signal assigns intent.

---

## 18. Relationship to CVD / Executed Flow

CVD measures executed aggressive flow.

Toxicity determines what fraction of the previous displayed touch can be accounted for by trades at that exact touch.

Useful combinations include:

- aggressive buy flow + ask fill fraction;
- aggressive sell flow + bid fill fraction;
- large aggressive flow + high same-price replenishment;
- large aggressive flow + touch retreat;
- weak aggressive flow + large net unexplained withdrawal.

These are observable configurations.

The combined interpretation belongs downstream.

---

## 19. Relationship to Hawkes / Event Intensity

Arrival intensity can be compared with attribution rates.

Examples:

\[
\frac{\lambda_{\text{sell}}}{Q^b_0}
\]

alongside:

\[
\dot E^b,\quad \dot W^b,\quad \dot R^b
\]

and equivalent ask-side quantities.

High trade-arrival intensity with persistent replenishment differs from high intensity with rapid touch retreat.

No behavioral conclusion is emitted.

---

## 20. Relationship to Price Response

Subsequent or contemporaneous price measurements may be combined with:

- fill fraction;
- withdrawal fraction;
- replenishment fraction;
- retreat fraction;
- their divergences and velocities;
- toxicity SNR.

Examples include measuring whether price moved after liquidity was executed, replenished, withdrawn, or retreated.

The toxicity signal does not claim that the liquidity event caused the price response.

---

## 21. Cross-Symbol Comparison

Absolute quantities MUST NOT be compared across arbitrary symbols as though their scales were equivalent.

The following MAY be compared across symbols when feed semantics and event attribution are equivalent:

- `touch_fill_fraction`;
- `net_withdrawal_fraction`;
- `retreat_fraction`;
- `net_replenishment_fraction`;
- their standardized historical divergences;
- event-time-normalized fractional rates;
- historical-path distances after standardization.

Raw quantity and raw quantity-rate comparisons require an explicitly comparable economic scale.

---

## 22. Feed Integrity and Attribution Validity

Attribution is only as valid as the bracket.

A bracket MUST be rejected or marked undefined when:

- trade-feed continuity is broken;
- book-feed continuity is broken;
- event ordering is ambiguous;
- the current observation precedes or equals the previous timestamp;
- symbol or venue identity changes;
- a reconnect or snapshot reset makes the two touches incomparable;
- trade-side semantics are unavailable;
- price precision prevents reliable equality at the venue's tick representation.

Price matching MUST use the venue's exact discrete price representation or tick-normalized integer price.

Floating-point approximate equality MUST NOT be used to decide whether a trade executed at the previous touch.

---

## 23. Invalid and Missing States

The signal MUST distinguish:

1. measured zero fill;
2. measured zero withdrawal;
3. measured zero replenishment;
4. measured zero retreat;
5. no previous comparable touch;
6. no valid trade bracket;
7. unresolved disposition after touch improvement;
8. unavailable historical baseline;
9. unavailable covariance for SNR;
10. feed-integrity failure.

Rules:

- no trades in a valid bracket means measured zero matched fill, not missing fill;
- no previous touch means attribution metrics are undefined;
- touch improvement without previous-level visibility means disposition metrics are undefined;
- zero previous quantity makes all fractions using \(Q_0\) undefined;
- missing or invalid observations are never converted to zero.

---

## 24. Explicit Non-Claims

The toxicity signal does not determine:

- whether an order was genuine;
- whether displayed liquidity was fake;
- whether a participant intended to trade;
- whether a withdrawal was deceptive;
- whether a pattern is spoofing;
- whether a market maker behaved defensively;
- whether liquidity is toxic;
- whether cancellation is malicious;
- whether replenishment is support or resistance;
- whether a fill is bullish or bearish;
- whether a recurring pattern will produce the same future outcome.

Those are downstream reasoning tasks.

---

## 25. Minimal Required Metric Set

A valid toxicity implementation SHOULD minimally publish:

- `previous_best_price:bid`;
- `previous_best_price:ask`;
- `best_price:bid`;
- `best_price:ask`;
- `previous_touch_quantity:bid`;
- `previous_touch_quantity:ask`;
- `touch_quantity:bid`;
- `touch_quantity:ask`;
- `touch_price_log_change:bid`;
- `touch_price_log_change:ask`;
- `bracket_trade_quantity`;
- `matched_touch_trade_quantity:bid`;
- `matched_touch_trade_quantity:ask`;
- `touch_fill_quantity:bid`;
- `touch_fill_quantity:ask`;
- `touch_fill_fraction:bid`;
- `touch_fill_fraction:ask`;
- `touch_fill_rate:bid`;
- `touch_fill_rate:ask`;
- `unfilled_residual_quantity:bid`;
- `unfilled_residual_quantity:ask`;
- `net_withdrawn_quantity:bid`;
- `net_withdrawn_quantity:ask`;
- `net_withdrawal_fraction:bid`;
- `net_withdrawal_fraction:ask`;
- `net_withdrawal_rate:bid`;
- `net_withdrawal_rate:ask`;
- `net_replenished_quantity:bid`;
- `net_replenished_quantity:ask`;
- `net_replenishment_fraction:bid`;
- `net_replenishment_fraction:ask`;
- `net_replenishment_rate:bid`;
- `net_replenishment_rate:ask`;
- `retreated_quantity:bid`;
- `retreated_quantity:ask`;
- `retreat_fraction:bid`;
- `retreat_fraction:ask`;
- `retreat_rate:bid`;
- `retreat_rate:ask`;
- `fill_fraction_baseline:bid`;
- `fill_fraction_baseline:ask`;
- `fill_fraction_zscore:bid`;
- `fill_fraction_zscore:ask`;
- `withdrawal_fraction_baseline:bid`;
- `withdrawal_fraction_baseline:ask`;
- `withdrawal_fraction_zscore:bid`;
- `withdrawal_fraction_zscore:ask`;
- `retreat_fraction_baseline:bid`;
- `retreat_fraction_baseline:ask`;
- `retreat_fraction_zscore:bid`;
- `retreat_fraction_zscore:ask`;
- `fill_fraction_velocity:bid`;
- `fill_fraction_velocity:ask`;
- `withdrawal_fraction_velocity:bid`;
- `withdrawal_fraction_velocity:ask`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `Maturity`;
- `SNR`.

Metrics whose prerequisites are not satisfied are explicitly undefined rather than emitted as fabricated zeros.
