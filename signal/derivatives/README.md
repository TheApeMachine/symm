# Derivatives Market-State Signal Specification

## 1. Purpose

The derivatives signal measures market facts that are intrinsic to a derivative instrument and its explicitly supplied reference prices.

It measures:

1. open interest and its event-time change;
2. derivative/reference basis;
3. basis change through time;
4. derivative and reference returns over aligned intervals;
5. liquidation notional, rate, share, and aggressor-side composition;
6. optional three-price basis geometry when derivative, index, and spot are genuinely distinct feeds;
7. causal historical divergence, recurrence, Maturity, and SNR.

The signal does not classify the market as:

- leveraged ignition;
- short squeeze;
- adverse leverage buildup;
- long deleveraging;
- derivatives decoupling;
- crowded;
- overleveraged;
- bullish;
- bearish.

Those are downstream interpretations.

---

## 2. First Principles

A derivatives signal is valuable only for measurements that are specific to the derivative market.

Ordinary aggressive trade flow belongs to the executed-flow/CVD signal.

Price-path correlation belongs to correlation.

Lead-lag belongs to lead-lag.

The derivatives signal therefore centers on:

\[
\boxed{
\text{Open Interest}
+
\text{Basis Geometry}
+
\text{Liquidation Flow}
}
\]

and preserves enough aligned price information for those quantities to be audited.

No arbitrary scaling constants are permitted.

Values MUST NOT be multiplied by constants such as `2`, `10`, `50`, or `100` merely to make them numerically resemble a score.

---

## 3. Inputs

### 3.1 Required derivative ticker inputs

When available:

| Input | Unit | Validity |
|---|---|---|
| derivative price | quote/base | finite, positive |
| open interest | venue-defined OI unit | finite, non-negative |
| event timestamp | time | causally ordered |
| instrument identity | derivative contract | explicit |

### 3.2 Required reference-price input for basis

At least one explicitly named reference price:

- index price;
- spot reference price;
- or another contractually defined reference.

The reference identity MUST be preserved.

A derivative price MUST NOT be silently reused as its own reference.

### 3.3 Required liquidation-trade inputs

For liquidation accounting:

| Input | Unit | Validity |
|---|---|---|
| execution price | quote/base | finite, positive |
| execution quantity | base/contracts | finite, positive |
| aggressor side | buy/sell | explicit |
| trade type | liquidation/non-liquidation | explicit |
| timestamp | time | causally ordered |

### 3.4 Optional distinct spot feed

Three-price geometry requires all three prices to be genuinely distinct observations:

\[
P_d=\text{derivative}
\]

\[
P_i=\text{index}
\]

\[
P_s=\text{spot}
\]

If no independent spot feed exists, spot-derived metrics are undefined.

The implementation MUST NOT set:

\[
P_s=P_i
\]

merely to populate a field.

---

## 4. Measurement Envelope

Every measurement contains:

- `From`;
- `At`;
- `Maturity`;
- `SNR`.

`At` is the event time at which the derivative state is valid.

`From` depends on the metric:

- point state: `From = At`;
- first-difference/rate: previous comparable observation;
- interval liquidation metrics: start of the retained execution interval;
- historical estimator: earliest retained observation with non-zero estimator weight.

A composite measurement uses the earliest `From` required by the metrics it publishes.

---

## 5. Open Interest

### 5.1 `open_interest`

\[
\boxed{
OI_t
}
\]

**Unit:** exactly the exchange's OI unit.

The unit MUST be preserved as provenance:

- contracts;
- base quantity;
- quote notional;
- or another venue-defined unit.

Open interest MUST NOT be marked dimensionless unless the venue actually supplies a dimensionless quantity.

---

### 5.2 `open_interest_log_change`

For:

\[
OI_t>0,\qquad OI_{t-1}>0
\]

define:

\[
\boxed{
\Delta \ell_{OI}
=
\log
\left(
\frac{OI_t}{OI_{t-1}}
\right)
}
\]

**Unit:** dimensionless.

This is a relative change, not a velocity.

---

### 5.3 `open_interest_growth_rate`

For:

\[
\Delta t=t-t_{-1}>0
\]

define:

\[
\boxed{
g_{OI}
=
\frac{
\log(OI_t/OI_{t-1})
}{
\Delta t
}
}
\]

**Unit:** inverse second.

This is the legitimate event-time rate of proportional OI change.

---

### 5.4 `open_interest_change`

When the raw OI unit itself is economically meaningful:

\[
\boxed{
\Delta OI=OI_t-OI_{t-1}
}
\]

**Unit:** the venue's OI unit.

---

### 5.5 Optional open-interest notional

Only when contract multiplier \(c\) is known:

\[
\boxed{
OI^{notional}
=
OI
\cdot c
\cdot P_d
}
\]

The exact contract specification MUST determine this conversion.

No generic multiplier is permitted.

---

## 6. Open-Interest Dynamics

A second difference is not an acceleration unless elapsed time is accounted for.

### 6.1 `open_interest_growth_velocity`

Fit a causal local regression:

\[
g_{OI,i}
=
a+\beta_{OI}(t_i-t)+\epsilon_i
\]

Then:

\[
\boxed{
v_{OI}=\beta_{OI}
}
\]

**Unit:** inverse second squared.

This replaces tick-to-tick differences of relative changes.

### 6.2 Slope SNR

When regression uncertainty is estimable:

\[
\boxed{
SNR_{\beta,OI}
=
\frac{\beta_{OI}^2}
{\operatorname{Var}(\beta_{OI})}
}
\]

Acceleration beyond this slope is omitted until independently justified.

---

## 7. Derivative / Reference Basis

Let:

\[
P_d=\text{derivative price}
\]

\[
P_r=\text{explicit reference price}
\]

### 7.1 `basis`

\[
\boxed{
b=
\frac{P_d-P_r}{P_r}
}
\]

**Unit:** dimensionless ratio.

If rendered as percent, conversion by exactly `100%` is a unit presentation conversion, not a scoring multiplier.

### 7.2 `log_basis`

\[
\boxed{
b_{\log}
=
\log
\left(
\frac{P_d}{P_r}
\right)
}
\]

**Unit:** dimensionless log ratio.

**Why:** log basis is symmetric under reciprocal price ratios and composes exactly across multiple reference legs.

Both representations MAY be published.

---

## 8. Basis Dynamics

### 8.1 `basis_change`

For two valid aligned observations:

\[
\boxed{
\Delta b=b_t-b_{t-1}
}
\]

This is a difference, not a velocity.

### 8.2 `basis_rate`

\[
\boxed{
\dot b=
\frac{b_t-b_{t-1}}{\Delta t}
}
\]

**Unit:** inverse second.

### 8.3 `basis_velocity`

For a more stable local derivative, fit:

\[
b_i=a+\beta_b(t_i-t)+\epsilon_i
\]

and publish:

\[
\boxed{
v_b=\beta_b
}
\]

**Unit:** inverse second.

The implementation SHOULD prefer this causal regression slope over raw irregular-event first differences.

---

## 9. Aligned Price Returns

For a common interval \([From,At]\):

\[
\boxed{
r_d=
\log
\left(
\frac{P_d(At)}{P_d(From)}
\right)
}
\]

\[
\boxed{
r_r=
\log
\left(
\frac{P_r(At)}{P_r(From)}
\right)
}
\]

### 9.1 `derivative_log_return`

\[
\boxed{r_d}
\]

### 9.2 `reference_log_return`

\[
\boxed{r_r}
\]

### 9.3 `return_gap`

\[
\boxed{
g_r=r_d-r_r
}
\]

By log identities:

\[
g_r
=
\Delta
\log
\left(
\frac{P_d}{P_r}
\right)
\]

so return gap is the change in log basis over the same interval.

This is a measured relative price path.

It is not "decoupling."

---

## 10. Optional Three-Price Basis Geometry

When independent derivative, index, and spot prices are available:

\[
P_d,\quad P_i,\quad P_s
\]

define:

### 10.1 `derivative_index_log_basis`

\[
\boxed{
b_{di}=
\log(P_d/P_i)
}
\]

### 10.2 `index_spot_log_basis`

\[
\boxed{
b_{is}=
\log(P_i/P_s)
}
\]

### 10.3 `derivative_spot_log_basis`

\[
\boxed{
b_{ds}=
\log(P_d/P_s)
}
\]

These satisfy the exact identity:

\[
\boxed{
b_{ds}=b_{di}+b_{is}
}
\]

when the observations are truly synchronized.

### 10.4 `basis_closure_error`

\[
\boxed{
e_b=
b_{ds}-b_{di}-b_{is}
}
\]

For perfectly aligned observations:

\[
e_b=0
\]

Non-zero closure error primarily exposes timestamp/as-of mismatch or inconsistent reference construction.

It is not a market-state score.

---

## 11. Liquidation Accounting

Liquidation trades are derivative-native events and remain in this signal.

For an observation interval:

\[
[From,At]
\]

let liquidation notional be:

\[
n_i=p_iq_i
\]

with aggressor side \(s_i\).

### 11.1 `liquidation_notional:buy`

\[
\boxed{
L_b=
\sum_{\text{liquidation},\,s_i=buy}
p_iq_i
}
\]

### 11.2 `liquidation_notional:sell`

\[
\boxed{
L_s=
\sum_{\text{liquidation},\,s_i=sell}
p_iq_i
}
\]

The names describe observed trade-side semantics only.

They MUST NOT be renamed "short liquidations" or "long liquidations" unless the venue feed explicitly provides that position-side fact.

### 11.3 `gross_liquidation_notional`

\[
\boxed{
L_g=L_b+L_s
}
\]

### 11.4 `net_liquidation_notional`

\[
\boxed{
L_n=L_b-L_s
}
\]

### 11.5 `liquidation_signed_fraction`

For:

\[
L_g>0
\]

define:

\[
\boxed{
\phi_L=
\frac{L_b-L_s}{L_b+L_s}
}
\]

**Range:** `[-1,1]`.

---

## 12. Liquidation Rate and Share

For interval duration:

\[
T=At-From>0
\]

### 12.1 `liquidation_notional_rate`

\[
\boxed{
\dot L=
\frac{L_g}{T}
}
\]

**Unit:** quote currency / second.

### 12.2 `gross_derivative_trade_notional`

Let:

\[
\boxed{
G=
\sum_{\text{all derivative trades in interval}}
p_iq_i
}
\]

### 12.3 `liquidation_share`

For:

\[
G>0
\]

define:

\[
\boxed{
q_L=
\frac{L_g}{G}
}
\]

**Range:** `[0,1]`.

This is the economically meaningful liquidation fraction of executed derivative notional over the same interval.

A single liquidation event MUST NOT be divided by its own notional and reported as a meaningful "intensity"; that construction collapses mechanically to one.

---

## 13. Causal Historical Baselines

The signal SHOULD maintain causal baselines for:

- open-interest growth rate;
- basis;
- return gap;
- liquidation notional rate;
- liquidation share.

The current observation is scored against prior estimators before the estimators update.

### 13.1 `open_interest_growth_baseline`

\[
\boxed{
\mu^{OI}_{t-}
=
E[g_{OI}]_{t-}
}
\]

### 13.2 `open_interest_growth_zscore`

\[
\boxed{
z_{OI}
=
\frac{
g_{OI}-\mu^{OI}_{t-}
}{
\sigma^{OI}_{t-}
}
}
\]

### 13.3 `basis_baseline`

\[
\boxed{
\mu^b_{t-}=E[b]_{t-}
}
\]

### 13.4 `basis_zscore`

\[
\boxed{
z_b=
\frac{b-\mu^b_{t-}}{\sigma^b_{t-}}
}
\]

### 13.5 `return_gap_zscore`

\[
\boxed{
z_g=
\frac{
g_r-\mu^g_{t-}
}{
\sigma^g_{t-}
}
}
\]

---

## 14. Positive Liquidation-Rate Baseline

For positive liquidation rates:

\[
\dot L>0
\]

model:

\[
y_L=\log\dot L
\]

against a causal positive-rate baseline.

Zero liquidation rate remains an explicit zero state.

For positive observations:

\[
\boxed{
z_L=
\frac{
\log\dot L-\mu^L_{t-}
}{
\sigma^L_{t-}
}
}
\]

A two-part/hurdle model MAY later be used if zero liquidation intervals dominate.

No arbitrary epsilon is inserted before taking logs.

---

## 15. Signal-to-Noise Ratio

Define the always-defined core state, where prerequisites exist:

\[
\boxed{
X_t=
\begin{bmatrix}
g_{OI,t}\\
b_t\\
g_{r,t}\\
q_{L,t}
\end{bmatrix}
}
\]

where:

- \(g_{OI}\) = OI growth rate;
- \(b\) = derivative/reference basis;
- \(g_r\) = derivative/reference return gap;
- \(q_L\) = liquidation share.

Let the causal baseline and residual covariance be:

\[
\mu_{t-},\qquad\Sigma_{t-}
\]

For the \(k\) currently defined dimensions:

\[
\delta_t=X_t-\mu_{t-}
\]

\[
\boxed{
SNR_t=
\frac{1}{k}
\delta_t^\top
\Sigma_{t-}^{-1}
\delta_t
}
\]

SNR is undefined until the relevant covariance is estimable and non-degenerate.

SNR measures distinguishability of the current derivatives state from its own historical noise.

It does not identify a squeeze, buildup, ignition, or deleveraging event.

---

## 16. Maturity

For historical estimator weights \(w_i\):

\[
\boxed{
N_{\mathrm{eff}}
=
\frac{(\sum_iw_i)^2}{\sum_iw_i^2}
}
\]

Then:

\[
\boxed{
Maturity=
\begin{cases}
0,&N_{\mathrm{eff}}\le1\\
1-\frac{1}{N_{\mathrm{eff}}},&N_{\mathrm{eff}}>1
\end{cases}
}
\]

Measurement-level maturity is the minimum maturity required by the joint SNR.

Direct point measurements such as current basis and OI remain valid even while historical quality is immature.

---

## 17. Temporal Dynamics

Recommended causal local-regression metrics:

- `open_interest_growth_velocity`;
- `basis_velocity`;
- `return_gap_velocity`;
- `liquidation_share_velocity`.

For any state \(x_i\):

\[
x_i=a+\beta_x(t_i-t)+\epsilon_i
\]

publish:

\[
\boxed{
v_x=\beta_x
}
\]

and optionally:

\[
\boxed{
SNR_{\beta_x}
=
\frac{\beta_x^2}{\operatorname{Var}(\beta_x)}
}
\]

when slope uncertainty is estimable.

---

## 18. Historical Recurrence

The signal MAY retain the standardized state path:

\[
\boxed{
Z_t=
\begin{bmatrix}
z_{OI,t}\\
z_{b,t}\\
z_{g,t}\\
z_{L,t}
\end{bmatrix}
}
\]

using only defined dimensions.

Recommended metrics:

- `historical_path_distance`;
- `historical_path_percentile`;
- `historical_match_from`.

These measure recurrence or novelty only.

The signal does not infer what followed prior matches.

---

## 19. Relationship to CVD

Ordinary derivative trade flow belongs to CVD.

Useful downstream combinations include:

- OI growth rate + signed net aggressive flow;
- basis change + net aggressive flow;
- liquidation share + executed-flow rate;
- return gap + flow-response residual.

The derivatives signal SHOULD NOT duplicate generic CVD classifications.

---

## 20. Relationship to Hawkes

Liquidation events may be represented as distinct Hawkes marks.

Useful combinations include:

- liquidation rate + liquidation-arrival intensity;
- liquidation share + branching structure;
- OI change + liquidation event clustering.

Arrival timing and liquidation notional remain separate measurements.

---

## 21. Relationship to Liquidity, Depthflow, and Toxicity

Useful downstream combinations include:

- liquidation rate versus touch capacity;
- OI growth versus displayed-depth change;
- basis movement versus book turnover;
- liquidation bursts versus touch retreat or withdrawal.

No causal claim is emitted.

---

## 22. Relationship to Correlation and Lead-Lag

Explicitly related derivative/reference pairs may be compared using:

- contemporaneous correlation;
- lead-lag seconds;
- basis;
- return gap;
- basis velocity.

Basis is not correlation.

Lead-lag is not basis.

They describe different geometry.

---

## 23. Cross-Symbol Comparability

Dimensionless metrics MAY be compared across compatible derivative contracts:

- basis;
- log basis;
- OI growth rate;
- OI growth z-score;
- liquidation share;
- liquidation signed fraction;
- standardized divergences;
- SNR.

Raw OI values are not comparable across contracts unless their contract units are economically harmonized.

Raw liquidation notional and rate require common quote-currency or explicit conversion.

---

## 24. Invalid and Missing States

The signal MUST distinguish:

1. no OI;
2. zero OI;
3. no previous OI;
4. no valid reference price;
5. no independent spot price;
6. valid zero basis;
7. no liquidations in a valid interval;
8. no derivative trades in an interval;
9. no valid interval duration;
10. unavailable covariance;
11. feed discontinuity;
12. mismatched timestamps between reference legs.

Rules:

- no liquidation in a valid interval means `gross_liquidation_notional = 0`;
- `liquidation_signed_fraction` is undefined when gross liquidation is zero;
- `liquidation_share` is undefined when gross derivative trade notional is zero;
- three-price metrics are undefined without three independent prices;
- missing values are never converted to zero merely to fill a measurement.

---

## 25. Explicit Non-Claims

The derivatives signal does not determine:

- leveraged ignition;
- short squeeze;
- long squeeze;
- leverage buildup;
- deleveraging;
- decoupling;
- crowding;
- liquidation cascade;
- price discovery;
- bullishness or bearishness;
- whether OI growth is healthy or dangerous;
- whether basis will converge.

Those belong downstream.

---

## 26. Minimal Required Metric Set

A valid derivatives implementation SHOULD minimally publish:

- `derivative_price`;
- `reference_price`;
- `reference_type`;
- `open_interest`;
- `open_interest_log_change`;
- `open_interest_growth_rate`;
- `open_interest_growth_velocity`;
- `basis`;
- `log_basis`;
- `basis_change`;
- `basis_rate`;
- `basis_velocity`;
- `derivative_log_return`;
- `reference_log_return`;
- `return_gap`;
- `liquidation_notional:buy`;
- `liquidation_notional:sell`;
- `gross_liquidation_notional`;
- `net_liquidation_notional`;
- `liquidation_signed_fraction`;
- `liquidation_notional_rate`;
- `gross_derivative_trade_notional`;
- `liquidation_share`;
- `open_interest_growth_baseline`;
- `open_interest_growth_zscore`;
- `basis_baseline`;
- `basis_zscore`;
- `return_gap_zscore`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `Maturity`;
- `SNR`.

Optional three-price metrics are emitted only when derivative, index, and spot feeds are genuinely distinct.
