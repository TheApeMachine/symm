# Pump/Dump (Legacy) / Volume-Clocked Activity Signal Specification

## 1. Naming

`pumpdump` is a legacy package name.

"Pump" and "dump" are market interpretations, not measurements.

The mathematically legitimate signal is:

\[
\boxed{
\text{Volume-Clocked Tape Activity + Spread + Midpoint Response}
}
\]

A future package rename such as `volumeclock`, `activity`, or `tapeactivity` is recommended.

The source MUST NOT emit pump/dump classifications.

---

## 2. Purpose

The signal measures one symbol's executed activity on an adaptive volume clock.

It measures:

1. raw trade quantity and notional;
2. completed volume-bar quantity and duration;
3. completed-bar volume/notional rate;
4. causal activity-rate baseline and ratio;
5. executable spread and relative spread;
6. spread baseline, ratio, and divergence;
7. midpoint response over each completed volume bar;
8. directional return components;
9. temporal dynamics, historical recurrence, Maturity, and SNR.

It does not classify:

- ignition;
- precursor;
- compression;
- exhaustion;
- pump;
- dump;
- breakout;
- fade.

---

## 3. Inputs

Each executed trade requires:

| Input | Unit | Validity |
|---|---|---|
| execution price | quote/base | finite, positive |
| execution quantity | base quantity | finite, positive |
| timestamp | time | causally ordered |

For spread and response metrics, the latest valid executable quote at or before the trade is required:

| Input | Unit | Validity |
|---|---|---|
| best bid | quote/base | finite, positive |
| best ask | quote/base | finite, greater than bid |
| quote timestamp | time | not after trade |

A trade without a valid touch may still advance pure tape accounting, but touch-dependent metrics are undefined.

---

## 4. Raw Trade Metrics

### 4.1 `trade_price`

\[
\boxed{p_i}
\]

### 4.2 `trade_quantity`

\[
\boxed{q_i}
\]

### 4.3 `trade_notional`

\[
\boxed{
n_i=p_iq_i
}
\]

### 4.4 `trade_interval_seconds`

For sequential trades:

\[
\boxed{
\Delta t_i=t_i-t_{i-1}
}
\]

when positive.

These raw observations are provenance for the adaptive clock.

---

## 5. Volume-Clock Target

The volume clock uses a causal robust scale derived from the symbol's own prior trade-quantity distribution.

Let prior retained positive trade quantities be:

\[
\mathcal{Q}_{t-}
\]

Define:

\[
\boxed{
Q^\ast_t=
\operatorname{median}
\mathcal{Q}_{t-}
}
\]

`Q*` is fixed when a new bar opens.

The current bar MUST NOT continuously resize its own closing threshold using quantities that arrive after the bar begins.

This avoids a moving target.

If no prior positive quantity distribution exists, no statistically contextualized volume bar exists yet.

A bootstrap first bar MAY be retained internally for estimator seeding, but it MUST be marked provisional and MUST NOT fabricate baseline-relative metrics.

---

## 6. Volume Bar

Let bar \(k\) open at:

\[
t_k^0
\]

and contain trades \(i\in\mathcal{B}_k\).

Accumulated quantity is:

\[
\boxed{
Q_k=
\sum_{i\in\mathcal{B}_k}q_i
}
\]

The bar closes at the first causally ordered event satisfying:

\[
\boxed{
Q_k\ge Q^\ast_k
}
\]

with positive elapsed duration.

The close trade is included in the completed bar.

No fractional allocation of one trade across bars is required unless the execution feed itself supports divisible event accounting and the implementation explicitly chooses it.

---

## 7. Completed-Bar Metrics

### 7.1 `volume_bar_target_quantity`

\[
\boxed{Q^\ast_k}
\]

### 7.2 `volume_bar_quantity`

\[
\boxed{Q_k}
\]

Because the final trade may overshoot the target:

\[
Q_k\ge Q^\ast_k
\]

### 7.3 `volume_bar_notional`

\[
\boxed{
N_k=
\sum_{i\in\mathcal{B}_k}p_iq_i
}
\]

### 7.4 `volume_bar_trade_count`

\[
\boxed{
C_k=|\mathcal{B}_k|
}
\]

### 7.5 `volume_bar_duration`

\[
\boxed{
T_k=t_k^1-t_k^0
}
\]

**Unit:** seconds.

---

## 8. Activity Rates

For:

\[
T_k>0
\]

### 8.1 `volume_rate`

\[
\boxed{
R^Q_k=
\frac{Q_k}{T_k}
}
\]

**Unit:** base quantity / second.

### 8.2 `notional_rate`

\[
\boxed{
R^N_k=
\frac{N_k}{T_k}
}
\]

**Unit:** quote currency / second.

### 8.3 `trade_rate`

\[
\boxed{
R^C_k=
\frac{C_k}{T_k}
}
\]

**Unit:** trades / second.

These metrics distinguish economic throughput, base-volume throughput, and event frequency.

---

## 9. Causal Activity Baseline

Positive rate variables are modeled multiplicatively.

For positive notional rate:

\[
y^N_k=\log R^N_k
\]

with causal baseline:

\[
\mu^N_{k-}
\]

### 9.1 `notional_rate_baseline`

\[
\boxed{
B^N_k=e^{\mu^N_{k-}}
}
\]

### 9.2 `notional_rate_ratio`

\[
\boxed{
A^N_k=
\frac{R^N_k}{B^N_k}
}
\]

### 9.3 `notional_rate_divergence`

\[
\boxed{
d^N_k=
\log(R^N_k/B^N_k)
}
\]

### 9.4 `notional_rate_zscore`

\[
\boxed{
z^N_k=
\frac{d^N_k}{\sigma^N_{k-}}
}
\]

Equivalent metrics MAY be maintained for base-volume rate.

The descriptive name `notional_rate_ratio` is preferred over `rvol` when the numerator is actually notional rate.

---

## 10. Executable Touch Metrics

For a valid quote:

\[
b_k=\text{best bid}
\]

\[
a_k=\text{best ask}
\]

### 10.1 `best_bid`

\[
\boxed{b_k}
\]

### 10.2 `best_ask`

\[
\boxed{a_k}
\]

### 10.3 `midpoint`

\[
\boxed{
m_k=
\frac{a_k+b_k}{2}
}
\]

### 10.4 `spread`

\[
\boxed{
s_k=a_k-b_k
}
\]

### 10.5 `relative_spread`

\[
\boxed{
r^s_k=
\frac{s_k}{m_k}
=
\frac{2(a_k-b_k)}{a_k+b_k}
}
\]

**Unit:** dimensionless.

The exact formula MUST be documented; `spread/(bid+ask)` and `spread/midpoint` differ by a factor of two and MUST NOT be silently conflated.

This specification uses `spread / midpoint`.

---

## 11. Spread Historical Context

For positive relative spread:

\[
x^s_k=\log r^s_k
\]

with causal baseline:

\[
\mu^s_{k-}
\]

### 11.1 `relative_spread_baseline`

\[
\boxed{
B^s_k=e^{\mu^s_{k-}}
}
\]

### 11.2 `spread_ratio`

\[
\boxed{
R^s_k=
\frac{r^s_k}{B^s_k}
}
\]

### 11.3 `spread_divergence`

\[
\boxed{
d^s_k=
\log(r^s_k/B^s_k)
}
\]

### 11.4 `spread_zscore`

\[
\boxed{
z^s_k=
\frac{d^s_k}{\sigma^s_{k-}}
}
\]

A ratio below one means the current spread is narrower than its causal baseline.

That fact is published directly.

It is not renamed "compression."

---

## 12. Midpoint Response

Response price uses quote midpoint, not trade execution price.

Let:

\[
m_k^0
\]

be the causal midpoint associated with the bar opening trade and:

\[
m_k^1
\]

the causal midpoint associated with the closing trade.

### 12.1 `midpoint:from`

\[
\boxed{m_k^0}
\]

### 12.2 `midpoint:at`

\[
\boxed{m_k^1}
\]

### 12.3 `midpoint_log_return`

\[
\boxed{
r_k=
\log
\left(
\frac{m_k^1}{m_k^0}
\right)
}
\]

### 12.4 `midpoint_return_rate`

\[
\boxed{
u_k=
\frac{r_k}{T_k}
}
\]

for positive duration.

This prevents aggressor-side bid/ask bounce in trade prints from masquerading as price response.

---

## 13. Directional Return Components

The signal MAY publish an exact decomposition:

\[
\boxed{
r_k^+=\max(r_k,0)
}
\]

\[
\boxed{
r_k^-=\max(-r_k,0)
}
\]

such that:

\[
\boxed{
r_k=r_k^+-r_k^-
}
\]

and:

\[
\boxed{
|r_k|=r_k^++r_k^-
}
\]

Recommended names:

- `positive_midpoint_return`;
- `negative_midpoint_return`.

These are mathematical components.

They are not buy/sell precursor scores.

---

## 14. Return Historical Context

Maintain a causal additive baseline for signed midpoint return:

\[
\mu^r_{k-}
\]

with residual scale:

\[
\sigma^r_{k-}
\]

### 14.1 `midpoint_return_baseline`

\[
\boxed{
\mu^r_{k-}
}
\]

### 14.2 `midpoint_return_divergence`

\[
\boxed{
d^r_k=
r_k-\mu^r_{k-}
}
\]

### 14.3 `midpoint_return_zscore`

\[
\boxed{
z^r_k=
\frac{d^r_k}{\sigma^r_{k-}}
}
\]

No directional bounded score is required.

---

## 15. Signal-to-Noise Ratio

Define the core completed-bar state:

\[
\boxed{
X_k=
\begin{bmatrix}
\log R^N_k\\
r_k\\
\log r^s_k
\end{bmatrix}
}
\]

Let causal baseline and residual covariance be:

\[
\mu_{k-},\qquad\Sigma_{k-}
\]

Then:

\[
\delta_k=X_k-\mu_{k-}
\]

\[
\boxed{
SNR_k=
\frac{1}{3}
\delta_k^\top
\Sigma_{k-}^{-1}
\delta_k
}
\]

SNR measures joint unusualness of:

- executed activity rate;
- midpoint response;
- executable spread.

It is not pump probability, ignition confidence, or hypothesis separation.

---

## 16. Maturity

For historical completed-bar weights \(w_i\):

\[
\boxed{
N_{\mathrm{eff}}
=
\frac{(\sum_iw_i)^2}
{\sum_iw_i^2}
}
\]

\[
\boxed{
Maturity=
\begin{cases}
0,&N_{\mathrm{eff}}\le1\\
1-\frac{1}{N_{\mathrm{eff}}},&N_{\mathrm{eff}}>1
\end{cases}
}
\]

A completed bar's raw measurements are valid before historical SNR matures.

---

## 17. Temporal Dynamics

Recommended causal local-regression metrics:

### 17.1 `notional_rate_velocity`

Fit:

\[
\log R^N_i
=
a+\beta_N(t_i-t)+\epsilon_i
\]

\[
\boxed{
v_N=\beta_N
}
\]

### 17.2 `spread_divergence_velocity`

Fit:

\[
d^s_i
=
a+\beta_s(t_i-t)+\epsilon_i
\]

### 17.3 `midpoint_return_velocity`

Fit:

\[
r_i=
a+\beta_r(t_i-t)+\epsilon_i
\]

Slope-specific SNR MAY be emitted when uncertainty is estimable.

No arbitrary acceleration score is required.

---

## 18. Historical Recurrence

The signal MAY retain the standardized completed-bar path:

\[
\boxed{
Z_k=
\begin{bmatrix}
z^N_k\\
z^r_k\\
z^s_k
\end{bmatrix}
}
\]

Recommended metrics:

- `historical_path_distance`;
- `historical_path_percentile`;
- `historical_match_from`.

A historical match is not a forecast of a repeated outcome.

---

## 19. Relationship to CVD

CVD measures direction and economic size of aggressive execution.

Volume-clock activity measures the rate and price/spread context of executed activity.

Useful downstream combinations include:

- notional-rate ratio + signed net fraction;
- midpoint return + flow-response residual;
- high activity rate + balanced flow;
- ordinary activity rate + highly one-sided flow.

No "ignition" label is emitted.

---

## 20. Relationship to Liquidity

Useful downstream combinations include:

- activity rate versus touch capacity;
- spread ratio versus depth ratio;
- midpoint return under shallow versus deep displayed liquidity.

The volume-clock signal does not infer whether a spread state is coiled or fragile.

---

## 21. Relationship to Depthflow and Toxicity

Useful combinations include:

- activity-rate divergence + book-turnover rate;
- spread divergence + touch withdrawal;
- midpoint response + replenishment;
- high executed activity with little displayed-book change.

No intent or manipulation classification is emitted.

---

## 22. Relationship to Hawkes

Hawkes measures event-arrival intensity.

Volume-clock activity measures completed economic throughput.

Useful distinctions include:

- high event intensity with small average trades;
- modest event intensity with large notional throughput;
- arrival clustering with widening or narrowing spread.

---

## 23. Relationship to Sentiment / Correlation

Useful downstream combinations include:

- local volume-rate divergence + cross-sectional median return;
- local midpoint response + cohort breadth;
- local activity SNR + pair correlation divergence.

This signal remains single-symbol and does not infer a market-wide story.

---

## 24. Cross-Symbol Comparability

Dimensionless metrics MAY be compared across compatible symbols:

- notional-rate ratio;
- spread ratio;
- standardized divergences;
- SNR;
- historical-path percentile.

Raw trade quantity and base-volume rate are not comparable across arbitrary symbols.

Raw quote notional rate requires a common quote currency or explicit conversion.

Completed-bar duration is a direct event-time measurement and may itself be compared when feed semantics are compatible.

---

## 25. Invalid and Missing States

The signal MUST distinguish:

1. no prior trade-quantity scale;
2. open but incomplete volume bar;
3. valid completed bar;
4. zero bar duration;
5. no executable quote;
6. locked/crossed book;
7. unavailable activity baseline;
8. unavailable spread baseline;
9. unavailable covariance;
10. feed discontinuity.

Rules:

- an incomplete bar is not a zero-rate bar;
- a completed bar with zero midpoint return has a valid zero return;
- touch-dependent metrics are undefined without a valid quote;
- zero relative spread cannot enter a log baseline;
- missing metrics are never replaced with precursor/exhaustion scores.

---

## 26. Explicit Non-Claims

The volume-clocked activity signal does not determine:

- pump;
- dump;
- ignition;
- precursor;
- compression;
- exhaustion;
- breakout;
- rejection;
- trend quality;
- manipulation;
- future direction.

Those are downstream reasoning tasks.

---

## 27. Minimal Required Metric Set

A valid implementation SHOULD minimally publish:

- `trade_price`;
- `trade_quantity`;
- `trade_notional`;
- `volume_bar_target_quantity`;
- `volume_bar_quantity`;
- `volume_bar_notional`;
- `volume_bar_trade_count`;
- `volume_bar_duration`;
- `volume_rate`;
- `notional_rate`;
- `trade_rate`;
- `notional_rate_baseline`;
- `notional_rate_ratio`;
- `notional_rate_divergence`;
- `notional_rate_zscore`;
- `best_bid`;
- `best_ask`;
- `midpoint`;
- `spread`;
- `relative_spread`;
- `relative_spread_baseline`;
- `spread_ratio`;
- `spread_divergence`;
- `spread_zscore`;
- `midpoint:from`;
- `midpoint:at`;
- `midpoint_log_return`;
- `midpoint_return_rate`;
- `positive_midpoint_return`;
- `negative_midpoint_return`;
- `midpoint_return_baseline`;
- `midpoint_return_zscore`;
- `notional_rate_velocity`;
- `spread_divergence_velocity`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `Maturity`;
- `SNR`.

No semantic evidence scores are part of the signal contract.
