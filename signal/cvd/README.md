# CVD / Executed Flow Signal Specification

## 1. Purpose

The CVD signal measures executed aggressive flow and the contemporaneous market-price response to that flow.

It answers:

1. How much aggressive buy and sell quantity/notional executed?
2. What was the signed net flow and how one-sided was it?
3. At what rate did execution occur?
4. How did the quote midpoint move over the same interval?
5. How does current flow and response differ from the market's own causal history?
6. Is the current flow-response path familiar or unusual relative to prior paths?

The signal does not classify flow as drive, absorption, starvation, balance, confirmation, rejection, buying pressure, or selling pressure.

---

## 2. First Principles

For trade \(i\), define:

\[
p_i=\text{execution price}
\]

\[
q_i=\text{executed base quantity}
\]

and aggressor sign:

\[
s_i=
\begin{cases}
+1,&\text{aggressive buy}\\
-1,&\text{aggressive sell}
\end{cases}
\]

Executed quote-currency notional is:

\[
\boxed{
n_i=p_iq_i
}
\]

Signed executed notional is:

\[
\boxed{
x_i=s_i n_i
}
\]

Signed executed base quantity is:

\[
\boxed{
v_i=s_i q_i
}
\]

For an observation interval containing trades \(i=1,\ldots,N\):

\[
\boxed{
B=\sum_{s_i=+1} n_i
}
\]

\[
\boxed{
S=\sum_{s_i=-1} n_i
}
\]

\[
\boxed{
G=B+S
}
\]

\[
\boxed{
\Delta=B-S=\sum_i x_i
}
\]

where:

- \(B\) = aggressive buy notional;
- \(S\) = aggressive sell notional;
- \(G\) = gross aggressive notional;
- \(\Delta\) = signed net aggressive notional.

The scale-free signed flow imbalance is:

\[
\boxed{
\phi=\frac{\Delta}{G}
}
\]

for \(G>0\).

Its range is:

\[
-1\le\phi\le1
\]

Execution price and response price are different observables and MUST remain separate.

Execution price determines traded notional.

Response price is the contemporaneous quote midpoint:

\[
\boxed{
m=\frac{bid+ask}{2}
}
\]

using the latest valid quote available at or before the relevant trade timestamp.

Midpoint response over the interval is:

\[
\boxed{
\rho=\log\left(\frac{m_{At}}{m_{From}}\right)
}
\]

This avoids using trade-price changes as the primary response measurement, which would mix market movement with bid/ask execution bounce.

---

## 3. Observation Interval

The signal operates over a causal retained trade path.

The interval-selection mechanism MUST satisfy the global signals specification:

- no future observations;
- no fixed arbitrary trade count or wall-clock horizon;
- retention derived from observed event timing and estimator support/stability;
- the current observation is measured before it updates its own historical baseline.

For a retained path containing at least one trade:

\[
From=t_1
\]

\[
At=t_N
\]

where \(t_1\) and \(t_N\) are the first and last retained trade timestamps.

The number of trades in the interval is published explicitly.

Metrics that require elapsed time are undefined when:

\[
At=From
\]

Metrics that require a price response are undefined until two causally ordered response midpoints exist.

---

## 4. Inputs

### 4.1 Required trade inputs

| Input             | Unit          | Validity         |
|-------------------|---------------|------------------|
| execution price   | quote/base    | finite, positive |
| executed quantity | base quantity | finite, positive |
| aggressor side    | buy / sell    | explicit         |
| trade timestamp   | time          | causally ordered |

### 4.2 Required quote inputs for response metrics

| Input           | Unit       | Validity                              |
|-----------------|------------|---------------------------------------|
| best bid        | quote/base | finite, positive                      |
| best ask        | quote/base | finite, positive and greater than bid |
| quote timestamp | time       | not after the trade it contextualizes |

A trade MAY still contribute to executed-flow accounting if a valid quote midpoint is unavailable.

Price-response metrics are then undefined for the affected interval.

### 4.3 Feed integrity

The signal MUST preserve trade ordering and MUST NOT double-count duplicated trades.

If aggressor-side semantics are absent or unreliable, signed flow metrics are undefined.

---

## 5. Measurement Envelope

Every measurement contains:

- `From`;
- `At`;
- `Maturity`;
- `SNR`.

### 5.1 `From`

`From` is the first trade timestamp contributing to the retained flow observation.

### 5.2 `At`

`At` is the most recent trade timestamp represented by the measurement.

### 5.3 `Maturity`

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

Direct executed-flow accounting remains valid before historical estimators mature.

Measurement-level maturity is the minimum maturity required for the joint SNR.

### 5.4 `SNR`

Define:

\[
g_t=\frac{G_t}{At-From}
\]

for a positive interval duration.

Let the causal baseline gross-notional rate be \(B^g_{t-}>0\), and define:

\[
d^g_t=
\log\left(
\frac{g_t}{B^g_{t-}}
\right)
\]

Let the causal baseline of signed net fraction be:

\[
\mu^\phi_{t-}
\]

with divergence:

\[
d^\phi_t=\phi_t-\mu^\phi_{t-}
\]

Let midpoint response rate be:

\[
u_t=\frac{\rho_t}{At-From}
\]

with causal baseline:

\[
\mu^u_{t-}
\]

and divergence:

\[
d^u_t=u_t-\mu^u_{t-}
\]

Define:

\[
D_t=
\begin{bmatrix}
d^g_t\\
d^\phi_t\\
d^u_t
\end{bmatrix}
\]

and let \(\Sigma_{t-}\) be the causal covariance of these residual dimensions.

Then:

\[
\boxed{
SNR_t=
\frac{1}{3}
D_t^\top
\Sigma_{t-}^{-1}
D_t
}
\]

SNR is undefined until all required components and the covariance model are estimable.

It measures joint departure from normal executed-flow and midpoint-response behavior.

It is not a probability or a conclusion about what the flow means.

---

## 6. Trade Count Metrics

### 6.1 `trade_count`

\[
\boxed{
N=N_b+N_s
}
\]

**Unit:** count.

**Meaning:** number of executions in the observation interval.

---

### 6.2 `trade_count:buy`

\[
\boxed{
N_b=\sum_i \mathbf{1}(s_i=+1)
}
\]

**Unit:** count.

---

### 6.3 `trade_count:sell`

\[
\boxed{
N_s=\sum_i \mathbf{1}(s_i=-1)
}
\]

**Unit:** count.

---

### 6.4 `signed_count_fraction`

For \(N>0\):

\[
\boxed{
\phi_N=
\frac{N_b-N_s}{N}
}
\]

**Range:** `[-1,1]`.

**Meaning:** aggressor imbalance by event count rather than economic size.

**Downstream use:** compare whether directional imbalance comes from many executions or from notional size.

It is intentionally separate from `signed_net_fraction`.

---

## 7. Executed Quantity Metrics

### 7.1 `executed_quantity:buy`

\[
\boxed{
Q_b=\sum_{s_i=+1}q_i
}
\]

**Unit:** base quantity.

---

### 7.2 `executed_quantity:sell`

\[
\boxed{
Q_s=\sum_{s_i=-1}q_i
}
\]

**Unit:** base quantity.

---

### 7.3 `gross_executed_quantity`

\[
\boxed{
Q_g=Q_b+Q_s
}
\]

**Unit:** base quantity.

---

### 7.4 `net_executed_quantity`

\[
\boxed{
Q_\Delta=Q_b-Q_s
}
\]

**Unit:** base quantity.

**Meaning:** signed base-volume delta over the observation interval.

---

## 8. Executed Notional Metrics

### 8.1 `aggressive_notional:buy`

\[
\boxed{
B=\sum_{s_i=+1}p_iq_i
}
\]

**Unit:** quote currency.

**Meaning:** economic size of aggressive buy executions.

---

### 8.2 `aggressive_notional:sell`

\[
\boxed{
S=\sum_{s_i=-1}p_iq_i
}
\]

**Unit:** quote currency.

---

### 8.3 `gross_notional`

\[
\boxed{
G=B+S
}
\]

**Unit:** quote currency.

**Meaning:** total aggressive executed notional, independent of direction.

---

### 8.4 `net_notional`

\[
\boxed{
\Delta=B-S
}
\]

**Unit:** quote currency.

**Meaning:** signed aggressive executed notional.

Positive values indicate more aggressive buy notional.

Negative values indicate more aggressive sell notional.

This is accounting, not a directional forecast.

---

### 8.5 `signed_net_fraction`

For \(G>0\):

\[
\boxed{
\phi=\frac{B-S}{B+S}
}
\]

**Range:** `[-1,1]`.

**Meaning:** signed share of gross executed notional that remains after buy and sell aggression offset each other.

Examples:

- `+1`: all observed aggressive notional was buy-side;
- `0`: equal buy and sell notional;
- `-1`: all observed aggressive notional was sell-side.

**Why:** dimensionless and scale-free.

**Downstream use:** compare directional execution imbalance across time and, when feed semantics are equivalent, across markets.

---

### 8.6 `mean_trade_notional`

For \(N>0\):

\[
\boxed{
\bar n=\frac{G}{N}
}
\]

**Unit:** quote currency.

**Meaning:** mean economic size of executions in the interval.

**Downstream use:** distinguish high activity caused by many small trades from high activity caused by fewer large trades.

---

## 9. Execution Rate Metrics

For:

\[
\Delta t=At-From>0
\]

### 9.1 `trade_rate`

\[
\boxed{
\lambda_N=\frac{N}{\Delta t}
}
\]

**Unit:** trades / second.

---

### 9.2 `gross_notional_rate`

\[
\boxed{
g=\frac{G}{\Delta t}
}
\]

**Unit:** quote currency / second.

**Meaning:** economic execution activity per unit event time.

---

### 9.3 `net_notional_rate`

\[
\boxed{
f=\frac{\Delta}{\Delta t}
}
\]

**Unit:** quote currency / second.

**Meaning:** signed aggressive notional accumulation rate.

---

### 9.4 `buy_notional_rate`

\[
\boxed{
g_b=\frac{B}{\Delta t}
}
\]

**Unit:** quote currency / second.

---

### 9.5 `sell_notional_rate`

\[
\boxed{
g_s=\frac{S}{\Delta t}
}
\]

**Unit:** quote currency / second.

---

## 10. Cumulative Volume Delta

CVD is a path quantity and therefore requires an explicit origin.

Let the stream epoch begin at \(t_e\).

### 10.1 `cumulative_volume_delta`

\[
\boxed{
C_Q(t)=
\sum_{t_e<t_i\le t}s_iq_i
}
\]

**Unit:** base quantity.

### 10.2 `cumulative_notional_delta`

\[
\boxed{
C_N(t)=
\sum_{t_e<t_i\le t}s_ip_iq_i
}
\]

**Unit:** quote currency.

### 10.3 Required provenance

The measurement MUST preserve:

\[
\boxed{
cvd\_epoch\_from=t_e
}
\]

A cumulative value is not intrinsically comparable across different epochs.

Downstream analysis SHOULD prefer interval differences:

\[
C(t_1)-C(t_0)
\]

which recover signed executed flow over a defined interval.

Cumulative CVD is not used directly in measurement SNR.

---

## 11. Response Price Metrics

### 11.1 `response_midpoint:from`

\[
\boxed{
m_0=\frac{bid_0+ask_0}{2}
}
\]

**Unit:** quote/base.

**Meaning:** quote midpoint associated causally with the first trade in the observation interval.

---

### 11.2 `response_midpoint:at`

\[
\boxed{
m_1=\frac{bid_1+ask_1}{2}
}
\]

**Unit:** quote/base.

---

### 11.3 `midpoint_log_return`

For \(m_0,m_1>0\):

\[
\boxed{
\rho=\log(m_1/m_0)
}
\]

**Unit:** dimensionless.

**Why:** price-scale-independent response measurement that is not contaminated by aggressor execution side.

---

### 11.4 `midpoint_return_rate`

For \(\Delta t>0\):

\[
\boxed{
u=\frac{\rho}{\Delta t}
}
\]

**Unit:** inverse second.

**Meaning:** event-time-normalized midpoint response.

---

### 11.5 `flow_aligned_midpoint_return`

For \(\Delta\neq0\):

\[
\boxed{
\rho_{\parallel}
=
\operatorname{sgn}(\Delta)\rho
}
\]

**Unit:** dimensionless.

**Meaning:** midpoint return expressed in the direction of net aggressive notional.

- positive: midpoint moved in the same direction as net aggression;
- negative: midpoint moved opposite net aggression;
- zero: no midpoint change.

**Non-claim:** alignment does not establish causation or confirmation.

---

### 11.6 `midpoint_response_per_net_notional`

For \(\Delta\neq0\):

\[
\boxed{
\eta=
\frac{\rho}{\Delta}
}
\]

**Unit:** inverse quote currency.

Equivalent form:

\[
\eta=
\frac{\rho_{\parallel}}
{|\Delta|}
\]

**Meaning:** signed midpoint response per unit of net aggressive notional.

Positive values indicate response aligned with net aggression.

Negative values indicate response opposed to net aggression.

Values near zero indicate little midpoint movement per unit net aggressive notional.

**Why:** it preserves the empirical flow-response relationship without labeling it.

**Downstream use:** compare against the same symbol's history and against contemporaneous liquidity.

**Caution:** the ratio becomes numerically large when net notional is close to zero. The raw denominator MUST therefore remain available; no arbitrary minimum threshold is inserted.

---

## 12. Historical Gross-Flow Baseline

Gross notional rate is positive and naturally multiplicative.

Let:

\[
g_t>0
\]

The causal baseline is maintained in log space:

\[
y^g_t=\log g_t
\]

with pre-observation estimate:

\[
\mu^g_{t-}
\]

### 12.1 `gross_notional_rate_baseline`

\[
\boxed{
B^g_t=e^{\mu^g_{t-}}
}
\]

**Unit:** quote currency / second.

---

### 12.2 `gross_notional_rate_ratio`

\[
\boxed{
R^g_t=
\frac{g_t}{B^g_t}
}
\]

**Unit:** dimensionless.

---

### 12.3 `gross_notional_rate_divergence`

\[
\boxed{
d^g_t=
\log(g_t/B^g_t)
}
\]

**Unit:** dimensionless log ratio.

---

### 12.4 `gross_notional_rate_zscore`

\[
\boxed{
z^g_t=
\frac{d^g_t}
{\sigma^g_{t-}}
}
\]

where \(\sigma^g\) is the causal residual noise scale.

---

## 13. Historical Directional-Flow Baseline

Signed net fraction is already bounded and dimensionless.

### 13.1 `signed_net_fraction_baseline`

\[
\boxed{
\mu^\phi_{t-}=E[\phi]_{t-}
}
\]

### 13.2 `signed_net_fraction_divergence`

\[
\boxed{
d^\phi_t=
\phi_t-\mu^\phi_{t-}
}
\]

### 13.3 `signed_net_fraction_zscore`

\[
\boxed{
z^\phi_t=
\frac{d^\phi_t}
{\sigma^\phi_{t-}}
}
\]

**Use:** distinguish a one-sided flow interval that is ordinary for the market from one that is historically unusual.

---

## 14. Historical Midpoint-Response Baseline

For midpoint return rate:

\[
u_t=\frac{\rho_t}{\Delta t}
\]

maintain causal baseline:

\[
\mu^u_{t-}
\]

and residual scale:

\[
\sigma^u_{t-}
\]

### 14.1 `midpoint_return_rate_baseline`

\[
\boxed{
\mu^u_{t-}
}
\]

### 14.2 `midpoint_return_rate_divergence`

\[
\boxed{
d^u_t=u_t-\mu^u_{t-}
}
\]

### 14.3 `midpoint_return_rate_zscore`

\[
\boxed{
z^u_t=
\frac{d^u_t}
{\sigma^u_{t-}}
}
\]

This describes price response only.

It does not say whether the response is appropriate for the observed flow.

---

## 15. Optional Causal Flow-Response Model

A signal MAY estimate the empirical relationship between signed net flow rate and midpoint response rate.

Using prior observations only:

\[
u_j=\alpha+\beta f_j+\epsilon_j
\]

where:

\[
f_j=\text{net notional rate}
\]

\[
u_j=\text{midpoint return rate}
\]

The regression uses the causal adaptive historical weights defined by the global signals specification.

### 15.1 `flow_response_intercept`

\[
\boxed{\alpha_{t-}}
\]

**Unit:** inverse second.

### 15.2 `flow_response_coefficient`

\[
\boxed{\beta_{t-}}
\]

**Unit:** inverse quote currency.

**Meaning:** fitted midpoint-return-rate response per unit net-notional rate.

This is a fitted parameter, not a trading conclusion.

### 15.3 `expected_midpoint_return_rate`

\[
\boxed{
\hat u_t=
\alpha_{t-}+\beta_{t-}f_t
}
\]

### 15.4 `flow_response_residual`

\[
\boxed{
\epsilon_t=
u_t-\hat u_t
}
\]

**Unit:** inverse second.

**Meaning:** amount by which the observed midpoint response differs from the prior empirical flow-response relationship.

Positive and negative values are purely residual directions.

### 15.5 `flow_response_residual_snr`

With prior residual variance:

\[
\sigma^2_{\epsilon,t-}
\]

define:

\[
\boxed{
SNR^\epsilon_t=
\frac{\epsilon_t^2}
{\sigma^2_{\epsilon,t-}}
}
\]

**Meaning:** unusualness of the current flow-response residual relative to prior model noise.

The model MUST be omitted when its support is insufficient.

---

## 16. Temporal Dynamics

### 16.1 `net_notional_rate_velocity`

For prior signed net-notional rates \(f_i\), fit:

\[
f_i=\alpha+\beta_f(t_i-t)+\epsilon_i
\]

Then:

\[
\boxed{
v_f=\beta_f
}
\]

**Unit:** quote currency / second².

**Meaning:** change in signed aggressive-flow rate through time.

---

### 16.2 `gross_notional_rate_velocity`

For positive gross rate, fit in log space:

\[
\log g_i=
\alpha+\beta_g(t_i-t)+\epsilon_i
\]

Then:

\[
\boxed{
v_g=\beta_g
}
\]

**Unit:** log-rate / second.

**Meaning:** multiplicative acceleration or deceleration of executed activity.

---

### 16.3 Slope SNR

For either fitted slope:

\[
\boxed{
SNR_\beta=
\frac{\beta^2}
{\operatorname{Var}(\beta)}
}
\]

MAY be emitted when the regression is estimable.

---

## 17. Historical Recurrence

The signal MAY retain the standardized path:

\[
Z_t=
\begin{bmatrix}
z^g_t\\
z^\phi_t\\
z^u_t
\end{bmatrix}
\]

and, when the optional response model is available:

\[
Z_t=
\begin{bmatrix}
z^g_t\\
z^\phi_t\\
z^u_t\\
\epsilon_t/\sigma_{\epsilon,t-}
\end{bmatrix}
\]

The current trajectory is compared with non-overlapping historical trajectories of equivalent causal support.

Recommended metrics:

### 17.1 `historical_path_distance`

Distance to the closest prior executed-flow trajectory.

### 17.2 `historical_path_percentile`

Empirical percentile of the nearest-match distance within retained history.

### 17.3 `historical_match_from`

Start time of the nearest prior trajectory.

No flow regime or market-state label is emitted.

---

## 18. Relationship to Liquidity

Liquidity measures displayed executable capacity.

CVD measures actual aggressive execution.

The most direct cross-signal ratios are:

### 18.1 Buy aggression relative to ask touch capacity

\[
\boxed{
L_b=
\frac{B}{D_a}
}
\]

where \(D_a\) is displayed ask touch notional over a properly aligned interval/reference.

### 18.2 Sell aggression relative to bid touch capacity

\[
\boxed{
L_s=
\frac{S}{D_b}
}
\]

These ratios describe executed demand relative to displayed immediate capacity.

The same aggressive notional has different mechanical significance under very different displayed depth.

### 18.3 Price response conditioned on liquidity

`midpoint_response_per_net_notional` may be compared with:

- ask/bid touch notional;
- depth ratio to baseline;
- spread divergence;
- liquidity SNR.

The signals preserve the measurements; downstream reasoning evaluates their joint meaning.

---

## 19. Relationship to Toxicity

Toxicity accounts for the disposition of previously displayed touch liquidity.

Useful downstream combinations include:

- aggressive buy notional + ask fill fraction;
- aggressive sell notional + bid fill fraction;
- net aggression + same-price replenishment;
- net aggression + touch retreat;
- large net aggression + net unexplained withdrawal;
- flow-response residual + replenishment fraction.

These combinations distinguish observable execution/liquidity configurations without assigning intent.

---

## 20. Relationship to Depthflow

Depthflow measures displayed-book mutation.

Useful combinations include:

- aggressive buy rate + ask-side displayed-flow rate;
- aggressive sell rate + bid-side displayed-flow rate;
- net executed flow + signed displayed-book flow;
- gross executed-flow rate + book-turnover rate;
- signed net fraction + touch/full-book imbalance gap.

For example, aggressive buying while ask depth replenishes is measurably different from aggressive buying while ask depth contracts.

No interpretation such as absorption, pressure, or confirmation is emitted by either signal.

---

## 21. Relationship to Hawkes

Hawkes measures arrival dynamics; CVD measures event sizes and economic flow.

Useful combinations include:

\[
\lambda_b,\lambda_s
\]

with:

\[
g_b,g_s
\]

and:

\[
\bar n=\frac{G}{N}
\]

This separates:

- many small aggressive events;
- few large aggressive events;
- self-exciting arrival bursts with ordinary notional;
- ordinary arrival intensity with unusually large notional.

---

## 22. Relationship to Derivatives

When derivative-market measurements are available, useful combinations include:

- signed net aggressive flow + open-interest change;
- gross aggressive flow + liquidation notional;
- midpoint response + basis change;
- flow-response residual + open-interest rate.

The CVD signal does not infer leverage, squeezing, deleveraging, or liquidation causality.

---

## 23. Relationship to Correlation / Sentiment

Price movement and cross-market measurements may be contextualized by:

- `signed_net_fraction`;
- `gross_notional_rate`;
- `net_notional_rate`;
- `flow_aligned_midpoint_return`;
- `flow_response_residual`;
- CVD SNR.

A correlated price move accompanied by strong local aggressive flow is a different measured configuration from the same price move with little local execution imbalance.

The interpretation remains downstream.

---

## 24. Cross-Symbol Comparison

The following MAY be compared across symbols when trade-side semantics and observation construction are equivalent:

- `signed_net_fraction`;
- `signed_count_fraction`;
- standardized historical divergences;
- standardized response residuals;
- SNR;
- historical-path distance after standardization.

The following MUST NOT be compared across arbitrary symbols as though they shared a common scale:

- raw base quantity;
- raw notional;
- gross notional rate;
- net notional rate;
- cumulative CVD;
- `midpoint_response_per_net_notional`.

Those quantities require same-symbol historical normalization or an explicitly comparable economic context.

---

## 25. Invalid and Missing States

The signal MUST distinguish:

1. measured zero net flow;
2. measured zero midpoint response;
3. no valid aggressor side;
4. no valid response quote;
5. one-trade interval with zero elapsed duration;
6. zero gross flow;
7. zero net notional;
8. unavailable historical baseline;
9. unavailable covariance;
10. response-model insufficiency;
11. feed discontinuity.

Rules:

- equal buy and sell notional produces `net_notional = 0`, not missing;
- `signed_net_fraction` is undefined only when gross notional is zero;
- `midpoint_response_per_net_notional` is undefined when net notional is zero;
- price-response metrics are undefined without two valid causal response midpoints;
- rates are undefined when elapsed duration is zero;
- SNR is undefined until its noise covariance is estimable;
- missing metrics are never fabricated as zero.

---

## 26. Explicit Non-Claims

The CVD signal does not determine:

- whether aggressive flow is driving price;
- whether flow is being absorbed;
- whether a participant is defending a level;
- whether flow confirms a move;
- whether the tape is starved;
- whether flow is balanced in a strategic sense;
- whether a market is bullish or bearish;
- whether net buying or selling will continue;
- whether price should have moved more or less;
- whether a flow-response residual is good or bad;
- whether a repeated flow pattern will produce the same future outcome.

Those are downstream reasoning tasks.

---

## 27. Minimal Required Metric Set

A valid CVD / executed-flow implementation SHOULD minimally publish:

- `trade_count`;
- `trade_count:buy`;
- `trade_count:sell`;
- `signed_count_fraction`;
- `executed_quantity:buy`;
- `executed_quantity:sell`;
- `gross_executed_quantity`;
- `net_executed_quantity`;
- `aggressive_notional:buy`;
- `aggressive_notional:sell`;
- `gross_notional`;
- `net_notional`;
- `signed_net_fraction`;
- `mean_trade_notional`;
- `trade_rate`;
- `gross_notional_rate`;
- `net_notional_rate`;
- `buy_notional_rate`;
- `sell_notional_rate`;
- `cumulative_volume_delta`;
- `cumulative_notional_delta`;
- `cvd_epoch_from`;
- `response_midpoint:from`;
- `response_midpoint:at`;
- `midpoint_log_return`;
- `midpoint_return_rate`;
- `flow_aligned_midpoint_return`;
- `midpoint_response_per_net_notional`;
- `gross_notional_rate_baseline`;
- `gross_notional_rate_ratio`;
- `gross_notional_rate_divergence`;
- `gross_notional_rate_zscore`;
- `signed_net_fraction_baseline`;
- `signed_net_fraction_divergence`;
- `signed_net_fraction_zscore`;
- `midpoint_return_rate_baseline`;
- `midpoint_return_rate_divergence`;
- `midpoint_return_rate_zscore`;
- `net_notional_rate_velocity`;
- `gross_notional_rate_velocity`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `Maturity`;
- `SNR`.

The causal flow-response model is optional and is emitted only when its statistical prerequisites are satisfied.
