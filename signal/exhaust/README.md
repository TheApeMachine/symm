# Exhaust / Microstructure Support-State Signal Specification

## 1. Architectural Status

`exhaust` is a legacy package name.

Under the measurement-only signal contract, "exhaustion" is not itself an observable market fact.

The existing package's concepts:

- mechanical;
- fragile;
- thermal;
- reversal;
- urgency;
- strength;
- category;

are interpretations of lower-level measurements and therefore belong downstream.

The preferred architecture is:

\[
\boxed{
\text{Liquidity}
+
\text{Depthflow}
+
\text{CVD}
+
\text{Price}
\rightarrow
\text{Reasoning / Category}
}
\]

with no separate `exhaust` signal at all.

If the `exhaust` source must remain for compatibility, it may exist only as a **Microstructure Support-State** measurement bundle exposing sufficient statistics.

It MUST NOT emit exit evidence or hypothetical position-side scores.

---

## 2. Purpose

The compatibility signal measures the joint state of microstructure facts that downstream reasoning may use when evaluating whether previously favorable conditions have changed.

It measures:

1. side-specific displayed depth relative to its own causal history;
2. total displayed-book depth relative to history;
3. spread relative to history;
4. aggressive executed-flow state;
5. recent executed-flow extrema and distance from them;
6. price return;
7. current and prior book imbalance;
8. temporal change, recurrence, Maturity, and SNR.

It does not know whether a position exists.

It does not know whether a long or short should be exited.

---

## 3. No Position Vocabulary

The signal MUST NOT use `buy` to mean "exit a long" or `sell` to mean "exit a short."

Sides refer only to measured market sides:

- `bid`;
- `ask`;
- `aggressive_buy`;
- `aggressive_sell`.

Position-side interpretation belongs to the consumer.

---

## 4. Preferred Data Source

The preferred implementation does not re-ingest market feeds and recompute parallel versions of existing measurements.

It consumes or composes the canonical facts already measured by:

- Liquidity;
- Depthflow;
- CVD / Executed Flow.

If a single compatibility measurement is required, it republishes aligned sufficient statistics with their original provenance.

The source MUST NOT silently alter their definitions.

---

## 5. Measurement Envelope

Every measurement contains:

- `From`;
- `At`;
- `Maturity`;
- `SNR`.

`At` is the common as-of time.

All included upstream facts MUST be causal as-of observations:

\[
t_{source}\le At
\]

Their ages SHOULD be preserved.

If one required source is too stale for the current alignment contract, dependent joint metrics are undefined.

---

## 6. Displayed Depth Metrics

Let:

\[
D_b=\text{displayed bid-side notional}
\]

\[
D_a=\text{displayed ask-side notional}
\]

The exact observation domain MUST be preserved:

- touch;
- full observed book;
- fixed venue depth;
- other explicit domain.

### 6.1 `displayed_depth_notional:bid`

\[
\boxed{D_b}
\]

### 6.2 `displayed_depth_notional:ask`

\[
\boxed{D_a}
\]

### 6.3 `displayed_depth_notional`

\[
\boxed{
D=D_b+D_a
}
\]

Absolute book depth is observation-domain dependent.

---

## 7. Causal Depth Baselines

Because depth is positive and multiplicative, use log-space baselines.

For side \(s\):

\[
x^D_s=\log D_s
\]

with causal baseline:

\[
\mu^D_{s,t-}
\]

### 7.1 `depth_baseline:bid`

\[
\boxed{
B_b=e^{\mu^D_{b,t-}}
}
\]

### 7.2 `depth_baseline:ask`

\[
\boxed{
B_a=e^{\mu^D_{a,t-}}
}
\]

### 7.3 `depth_ratio:bid`

\[
\boxed{
R_b=\frac{D_b}{B_b}
}
\]

### 7.4 `depth_ratio:ask`

\[
\boxed{
R_a=\frac{D_a}{B_a}
}
\]

### 7.5 `depth_divergence:bid`

\[
\boxed{
d_b=\log(D_b/B_b)
}
\]

### 7.6 `depth_divergence:ask`

\[
\boxed{
d_a=\log(D_a/B_a)
}
\]

### 7.7 `depth_zscore:{bid,ask}`

\[
\boxed{
z_s=
\frac{d_s}{\sigma^D_{s,t-}}
}
\]

No directional "support" or "collapse" label is attached.

---

## 8. Total Book Depth Context

For:

\[
D=D_b+D_a>0
\]

maintain causal log baseline:

\[
B_D=e^{\mu^D_{t-}}
\]

### 8.1 `total_depth_baseline`

\[
\boxed{B_D}
\]

### 8.2 `total_depth_ratio`

\[
\boxed{
R_D=\frac{D}{B_D}
}
\]

### 8.3 `total_depth_zscore`

\[
\boxed{
z_D=
\frac{
\log(D/B_D)
}{
\sigma^D_{t-}
}
}
\]

This replaces semantic "mechanical thinning" scores.

---

## 9. Spread State

Let:

\[
b=\text{best bid}
\]

\[
a=\text{best ask}
\]

\[
m=\frac{a+b}{2}
\]

\[
s=a-b
\]

\[
r_s=\frac{s}{m}
\]

for a valid uncrossed positive book.

Recommended metrics:

- `spread`;
- `relative_spread`;
- `relative_spread_baseline`;
- `spread_ratio`;
- `spread_divergence`;
- `spread_zscore`.

For positive relative spread:

\[
\boxed{
R_s=
\frac{r_s}{B_s}
}
\]

\[
\boxed{
d_s=\log(r_s/B_s)
}
\]

\[
\boxed{
z_s=
\frac{d_s}{\sigma^s_{t-}}
}
\]

This replaces "fragile."

---

## 10. Executed-Flow State

The canonical executed-flow signal supplies:

\[
B=\text{aggressive buy notional}
\]

\[
S=\text{aggressive sell notional}
\]

\[
G=B+S
\]

\[
\phi=
\frac{B-S}{B+S}
\]

for \(G>0\).

Recommended compatibility metrics:

- `aggressive_notional:buy`;
- `aggressive_notional:sell`;
- `gross_notional`;
- `signed_net_fraction`;
- `gross_notional_rate`;
- `net_notional_rate`.

These are measured facts.

The signal MUST NOT rename them "pressure supporting a long" or "pressure supporting a short."

---

## 11. Flow Historical Context

Maintain a causal baseline for:

\[
\phi_t
\]

### 11.1 `signed_net_fraction_baseline`

\[
\boxed{
\mu^\phi_{t-}
}
\]

### 11.2 `signed_net_fraction_zscore`

\[
\boxed{
z_\phi=
\frac{
\phi-\mu^\phi_{t-}
}{
\sigma^\phi_{t-}
}
}
\]

This is historical directional-flow departure.

---

## 12. Causal Flow Extrema

When extrema are useful, they MUST be prior retained extrema.

Let retained causal history before current observation be \(\mathcal{H}_{t-}\).

### 12.1 `signed_net_fraction_prior_max`

\[
\boxed{
\phi_{\max,t-}
=
\max_{\tau\in\mathcal{H}_{t-}}
\phi_\tau
}
\]

### 12.2 `signed_net_fraction_prior_min`

\[
\boxed{
\phi_{\min,t-}
=
\min_{\tau\in\mathcal{H}_{t-}}
\phi_\tau
}
\]

### 12.3 `distance_from_prior_max`

\[
\boxed{
d_{\max}
=
\phi_t-\phi_{\max,t-}
}
\]

### 12.4 `distance_from_prior_min`

\[
\boxed{
d_{\min}
=
\phi_t-\phi_{\min,t-}
}
\]

The current observation is compared with the prior extremum before the extremum updates.

A current new maximum therefore produces:

\[
d_{\max}>0
\]

rather than erasing its own departure by redefining the denominator first.

No "pressure fade" interpretation is emitted.

---

## 13. Price Response

Use quote midpoint rather than trade execution price when measuring market response.

For aligned interval:

\[
\boxed{
r=
\log
\left(
\frac{m_{At}}{m_{From}}
\right)
}
\]

Recommended metrics:

- `midpoint:from`;
- `midpoint:at`;
- `midpoint_log_return`;
- `midpoint_return_rate`.

Directional decomposition MAY be published:

\[
\boxed{
r^+=\max(r,0)
}
\]

\[
\boxed{
r^-=\max(-r,0)
}
\]

These are components, not rejection scores.

---

## 14. Book Imbalance

Let bid and ask observed book notionals be:

\[
D_b,\quad D_a
\]

### 14.1 `book_imbalance`

\[
\boxed{
I_t=
\frac{D_b-D_a}{D_b+D_a}
}
\]

### 14.2 `previous_book_imbalance`

\[
\boxed{
I_{t-1}
}
\]

### 14.3 `book_imbalance_change`

\[
\boxed{
\Delta I=I_t-I_{t-1}
}
\]

### 14.4 `book_imbalance_baseline`

\[
\boxed{
\mu^I_{t-}
}
\]

### 14.5 `book_imbalance_zscore`

\[
\boxed{
z_I=
\frac{
I_t-\mu^I_{t-}
}{
\sigma^I_{t-}
}
}
\]

Publishing current and previous imbalance preserves any sign crossing as a fact.

The signal MUST NOT label it "reversal."

---

## 15. Signal-to-Noise Ratio

If this compatibility composite is retained, define its core joint state as:

\[
\boxed{
X_t=
\begin{bmatrix}
\log R_b\\
\log R_a\\
\log R_s\\
\phi_t\\
I_t
\end{bmatrix}
}
\]

where defined.

Let causal historical state be:

\[
\mu_{t-},\quad\Sigma_{t-}
\]

For \(k\) defined dimensions:

\[
\delta_t=X_t-\mu_{t-}
\]

\[
\boxed{
SNR=
\frac{1}{k}
\delta_t^\top
\Sigma_{t-}^{-1}
\delta_t
}
\]

SNR measures unusualness of the joint microstructure state.

It is not exit urgency.

---

## 16. Maturity

For causal estimator weights \(w_i\):

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

When the composite depends on several upstream mature estimators, measurement maturity is the minimum maturity required by the joint SNR.

---

## 17. Temporal Dynamics

Recommended causal local-regression measurements:

- `depth_divergence_velocity:bid`;
- `depth_divergence_velocity:ask`;
- `spread_divergence_velocity`;
- `signed_net_fraction_velocity`;
- `book_imbalance_velocity`.

For:

\[
x_i=a+\beta(t_i-t)+\epsilon_i
\]

publish:

\[
\boxed{v_x=\beta}
\]

and optional slope SNR when uncertainty is estimable.

---

## 18. Historical Recurrence

The signal MAY compare standardized joint-state trajectories:

\[
\boxed{
Z_t=
\begin{bmatrix}
z_{D_b}\\
z_{D_a}\\
z_s\\
z_\phi\\
z_I
\end{bmatrix}
}
\]

Recommended metrics:

- `historical_path_distance`;
- `historical_path_percentile`;
- `historical_match_from`.

No recurrence outcome is inferred.

---

## 19. Relationship to Liquidity

Liquidity is the canonical source for:

- touch capacity;
- spread;
- depth ratios;
- liquidity SNR.

The compatibility exhaust source SHOULD reuse those facts rather than recomputing alternative versions.

---

## 20. Relationship to Depthflow

Depthflow is the canonical source for:

- full-book imbalance;
- book turnover;
- additions/removals;
- resolution gap;
- imbalance dynamics.

These are direct inputs to any downstream reasoning about changing microstructure support.

---

## 21. Relationship to CVD

CVD is the canonical source for:

- aggressive buy/sell notional;
- signed net fraction;
- gross and net rates;
- midpoint response.

The exhaust compatibility source MUST NOT create a second contradictory definition of "pressure."

---

## 22. Relationship to Toxicity

Toxicity adds disposition accounting:

- fill;
- withdrawal;
- replenishment;
- retreat.

These may be combined downstream with depth and flow changes.

No intent or position conclusion is emitted.

---

## 23. Explicit Non-Claims

The exhaust / support-state signal does not determine:

- whether a move is exhausted;
- whether a long should exit;
- whether a short should exit;
- mechanical exhaustion;
- fragility;
- thermal exhaustion;
- reversal;
- urgency;
- strength;
- position safety;
- stop placement;
- whether market structure still "supports" a thesis.

Those are downstream reasoning tasks.

---

## 24. Minimal Compatibility Metric Set

If the package remains, it SHOULD minimally expose:

- `displayed_depth_notional:bid`;
- `displayed_depth_notional:ask`;
- `displayed_depth_notional`;
- `depth_baseline:bid`;
- `depth_baseline:ask`;
- `depth_ratio:bid`;
- `depth_ratio:ask`;
- `depth_zscore:bid`;
- `depth_zscore:ask`;
- `total_depth_ratio`;
- `total_depth_zscore`;
- `spread`;
- `relative_spread`;
- `relative_spread_baseline`;
- `spread_ratio`;
- `spread_zscore`;
- `signed_net_fraction`;
- `signed_net_fraction_baseline`;
- `signed_net_fraction_zscore`;
- `signed_net_fraction_prior_max`;
- `signed_net_fraction_prior_min`;
- `distance_from_prior_max`;
- `distance_from_prior_min`;
- `midpoint_log_return`;
- `book_imbalance`;
- `previous_book_imbalance`;
- `book_imbalance_change`;
- `book_imbalance_zscore`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `Maturity`;
- `SNR`.

The preferred implementation is still to remove this redundant signal and let downstream reasoning consume the canonical source measurements directly.
