# Liquidity Signal Specification

## 1. Purpose

The liquidity signal measures displayed executable capacity at the bid and ask, the cost separating those executable prices, how those quantities differ from their own causal historical baselines, how those divergences are evolving, and how familiar or unusual the current liquidity trajectory is relative to the symbol's own history.

When multi-level book data is available, the signal also measures scale-free book morphology. These structural metrics are designed to be comparable across symbols without comparing absolute liquidity levels.

The signal does not classify liquidity as good, bad, scarce, organic, synthetic, spoofed, healthy, or risky.

---

## 2. First Principles

At event time \(t\), let:

$$
b_t = \text{best bid price}
$$

$$
a_t = \text{best ask price}
$$

$$
q^b_t = \text{quantity resting at the best bid}
$$

$$
q^a_t = \text{quantity resting at the best ask}
$$

Immediate displayed capacity is directional.

A market sell executes against the bid:

$$
\boxed{D^b_t=b_tq^b_t}
$$

A market buy executes against the ask:

$$
\boxed{D^a_t=a_tq^a_t}
$$

These are quote-currency notionals.

The cost geometry of the touch is described separately:

$$
m_t=\frac{a_t+b_t}{2}
$$

$$
s_t=a_t-b_t
$$

$$
\boxed{r^s_t=\frac{s_t}{m_t}}
$$

Capacity and cost MUST remain separate measurements.

Absolute depth is interpreted primarily against the same market's own causal history. Cross-symbol comparison of absolute notional depth is not part of this signal.

Cross-symbol comparison is reserved for dimensionless book morphology.

---

## 3. Inputs

### 3.1 Required touch inputs

| Input                | Unit          | Validity                                                           |
|----------------------|---------------|--------------------------------------------------------------------|
| `best_bid_price`     | quote/base    | finite, positive                                                   |
| `best_ask_price`     | quote/base    | finite, positive, greater than bid for a normal uncrossed snapshot |
| `touch_quantity:bid` | base quantity | finite, non-negative                                               |
| `touch_quantity:ask` | base quantity | finite, non-negative                                               |
| event timestamp      | time          | monotonic per symbol                                               |

An explicitly observed zero quantity is different from a missing quantity.

Missing or invalid fields MUST NOT be converted to zero depth.

### 3.2 Optional full-book inputs

For each visible bid and ask level:

$$
(p_i,q_i)
$$

The full-book snapshot MUST be internally consistent at `At`.

Venue tick size and lot size SHOULD be carried as provenance when morphology metrics depend on price or quantity discretization.

---

## 4. Measurement Envelope

### 4.1 `At`

`At` is the event timestamp of the book snapshot.

### 4.2 `From`

For touch-only measurements without historical context:

$$
From=At
$$

For baseline, momentum, SNR, or recurrence measurements, `From` is the earliest retained observation contributing non-zero weight to the joint liquidity estimator or current trajectory.

### 4.3 `Maturity`

The joint liquidity estimator uses effective sample size:

$$
N_{\mathrm{eff}}=\frac{(\sum_iw_i)^2}{\sum_iw_i^2}
$$

and:

$$
\boxed{
Maturity=
\begin{cases}
0,&N_{\mathrm{eff}}\le1\\[4pt]
1-\frac{1}{N_{\mathrm{eff}}},&N_{\mathrm{eff}}>1
\end{cases}
}
$$

If separate bid, ask, and spread estimators have materially different support, per-component maturity SHOULD be retained and the measurement-level maturity is the minimum maturity required for the joint SNR.

### 4.4 `SNR`

Define the positive state vector:

$$
X_t=
\begin{bmatrix}
\log D^b_t\\
\log D^a_t\\
\log r^s_t
\end{bmatrix}
$$

Let the causal pre-observation baseline be:

$$
\mu_{t-}
$$

and the causal residual covariance be:

$$
\Sigma_{t-}
$$

Then:

$$
\delta_t=X_t-\mu_{t-}
$$

and the measurement SNR is:

$$
\boxed{SNR_t=\frac{1}{3}\delta_t^\top\Sigma_{t-}^{-1}\delta_t}
$$

This measures joint departure power while accounting for normal covariance between bid depth, ask depth, and spread.

`SNR` is undefined until \(\Sigma\) is estimable and sufficiently non-degenerate.

No arbitrary epsilon is added to force an answer.

---

## 5. Core Touch Metrics

### 5.1 `best_bid_price`

**Meaning:** highest displayed executable bid.

$$
P_b=b_t
$$

**Unit:** quote/base.

**Why:** raw execution provenance.

**Downstream use:** execution context; alignment with trade, toxicity, and price-response measurements.

---

### 5.2 `best_ask_price`

**Meaning:** lowest displayed executable ask.

$$
P_a=a_t
$$

**Unit:** quote/base.

**Why:** raw execution provenance.

**Downstream use:** execution context; alignment with trade, toxicity, and price-response measurements.

---

### 5.3 `touch_quantity:bid`

**Meaning:** base quantity displayed at the best bid.

$$
Q_b=q^b_t
$$

**Unit:** base quantity.

**Why:** preserves the physical resting amount independently of price.

**Downstream use:** fill/cancel/replenishment accounting and venue-level order-book analysis.

---

### 5.4 `touch_quantity:ask`

$$
Q_a=q^a_t
$$

Same contract as bid quantity.

---

### 5.5 `touch_notional:bid`

**Meaning:** quote-currency notional immediately displayed to market sellers.

$$
\boxed{D_b=b_tq^b_t}
$$

**Unit:** quote currency.

**Why:** uses the actual price at which the displayed bid quantity is executable.

**Downstream use:** compare aggressive sell flow, fills, cancellations, or price response with available displayed bid capacity.

---

### 5.6 `touch_notional:ask`

**Meaning:** quote-currency notional immediately displayed to market buyers.

$$
\boxed{D_a=a_tq^a_t}
$$

**Unit:** quote currency.

**Downstream use:** compare aggressive buy flow, fills, cancellations, or price response with available displayed ask capacity.

---

### 5.7 `midpoint`

$$
\boxed{m_t=\frac{a_t+b_t}{2}}
$$

**Unit:** quote/base.

**Why:** side-neutral price reference.

**Downstream use:** relative spread, book-distance normalization, and response-price measurements.

---

### 5.8 `spread`

$$
\boxed{s_t=a_t-b_t}
$$

**Unit:** quote/base.

**Why:** direct absolute separation between executable prices.

**Downstream use:** execution-cost context and historical spread comparison.

---

### 5.9 `relative_spread`

$$
\boxed{r^s_t=\frac{a_t-b_t}{(a_t+b_t)/2}}
$$

**Unit:** dimensionless, non-negative.

**Why:** removes nominal price scale while retaining the geometry of executable price separation.

**Downstream use:** within-symbol historical comparison and cross-symbol structural context.

---

### 5.10 `two_sided_touch_notional`

$$
\boxed{D^{2s}_t=\min(D^b_t,D^a_t)}
$$

**Unit:** quote currency.

**Meaning:** notional amount for which both immediate directions have at least that much displayed capacity.

**Why:** the minimum of directional notionals has a precise two-sided interpretation.

**Downstream use:** coarse lower bound on balanced touch capacity.

It MUST NOT replace the side-specific depth metrics.

---

### 5.11 `touch_notional_imbalance`

$$
\boxed{I_t=\frac{D^b_t-D^a_t}{D^b_t+D^a_t}}
$$

when \(D^b_t+D^a_t>0\).

**Unit / range:** dimensionless, `[-1,1]`.

**Why:** scale-free current asymmetry in displayed touch notional.

**Downstream use:** compare touch asymmetry with deeper-book morphology, aggressive flow, or subsequent price response.

**Non-claim:** it does not imply pressure, support, resistance, or intent.

---

## 6. Historical Baseline Metrics

Depth is positive and naturally multiplicative, so historical depth estimation operates in log space.

For side \(s\in\{b,a\}\):

$$
x^s_t=\log D^s_t
$$

Let the causal baseline be:

$$
\mu^s_{t-}
$$

and causal residual noise scale:

$$
\sigma^s_{t-}
$$

### 6.1 `touch_notional_baseline:{bid,ask}`

$$
\boxed{B^s_t=e^{\mu^s_{t-}}}
$$

**Unit:** quote currency.

**Meaning:** causal central estimate of normal displayed touch notional for that side.

**Downstream use:** intuitive historical reference.

---

### 6.2 `depth_ratio:{bid,ask}`

$$
\boxed{R^s_t=\frac{D^s_t}{B^s_t}}
$$

**Unit:** dimensionless.

Interpretation:

* `1`: at baseline;
* `0.5`: half baseline;
* `2`: twice baseline.

**Why:** directly interpretable multiplicative departure.

---

### 6.3 `depth_divergence:{bid,ask}`

$$
\boxed{d^s_t=\log D^s_t-\mu^s_{t-}=\log R^s_t}
$$

**Unit:** dimensionless log ratio.

**Why:** reciprocal multiplicative changes are symmetric.

**Downstream use:** trajectory analysis, velocity, recurrence, and multivariate SNR.

---

### 6.4 `depth_noise_scale:{bid,ask}`

$$
\boxed{\sigma^s_{t-}=\sqrt{E[(d^s)^2]_{t-}}}
$$

using a causal, event-time-weighted residual estimator.

**Unit:** dimensionless log ratio.

**Meaning:** normal historical variability around the side's depth baseline.

**Downstream use:** distinguish a large percentage change that is ordinary for the market from one that is statistically unusual.

---

### 6.5 `depth_zscore:{bid,ask}`

$$
\boxed{z^s_t=\frac{d^s_t}{\sigma^s_{t-}}}
$$

**Unit:** dimensionless.

**Why:** standardized departure using the market's own empirical noise.

**Undefined when:** the historical noise scale is not estimable or is degenerate.

---

## 7. Spread Historical Metrics

Because relative spread is strictly positive for a valid uncrossed book, use:

$$
y_t=\log r^s_t
$$

with causal baseline \(\mu^r_{t-}\) and noise \(\sigma^r_{t-}\).

### 7.1 `relative_spread_baseline`

$$
\boxed{B^r_t=e^{\mu^r_{t-}}}
$$

### 7.2 `spread_ratio`

$$
\boxed{R^r_t=\frac{r^s_t}{B^r_t}}
$$

### 7.3 `spread_divergence`

$$
\boxed{d^r_t=\log r^s_t-\mu^r_{t-}}
$$

### 7.4 `spread_zscore`

$$
\boxed{z^r_t=\frac{d^r_t}{\sigma^r_{t-}}}
$$

These metrics describe how current executable-price separation differs from its own historical state.

They do not imply fragility, stress, or opportunity.

---

## 8. Baseline and Noise Model

The baseline and covariance estimators MUST be causal.

For each observation:

1. read the pre-observation baseline and covariance;
2. calculate current ratios, divergences, z-scores, and SNR;
3. publish the measurement;
4. update the baseline, residual dispersion, and covariance with the current observation.

Estimator decay SHOULD be defined on event time.

The effective horizon SHOULD be derived from observed cadence and estimator support rather than a fixed wall-clock window.

---

## 9. Divergence Momentum

The current divergence does not reveal whether liquidity is moving farther from baseline or returning toward it.

For each divergence series:

$$
d^b_t,\quad d^a_t,\quad d^r_t
$$

fit a causal local time regression over the estimator's derived horizon:

$$
d_i=\alpha+\beta(t_i-t)+\epsilon_i
$$

### 9.1 `divergence_velocity:bid`

$$
\boxed{v^b_t=\beta_b}
$$

### 9.2 `divergence_velocity:ask`

$$
\boxed{v^a_t=\beta_a}
$$

### 9.3 `spread_divergence_velocity`

$$
\boxed{v^r_t=\beta_r}
$$

**Unit:** log-divergence per second.

**Why:** slope over event time is meaningful under irregular update cadence; raw tick-to-tick differences are not.

### 9.4 `divergence_velocity_snr:*`

When the regression has sufficient support:

$$
\boxed{SNR_\beta=\frac{\beta^2}{\operatorname{Var}(\beta)}}
$$

**Meaning:** trend power relative to the regression uncertainty of the slope.

**Downstream use:** distinguish persistent divergence movement from noisy oscillation.

No directional label is attached.

---

## 10. Historical Recurrence

The signal SHOULD retain the standardized liquidity-state path:

$$
Z_t=
\begin{bmatrix}
z^b_t\\
z^a_t\\
z^r_t
\end{bmatrix}
$$

over the same causally derived horizon used for local dynamics.

The present trajectory is compared with non-overlapping historical trajectories of equivalent derived duration.

A multivariate motif/discord method such as a matrix profile MAY be used.

### 10.1 `historical_path_distance`

**Meaning:** distance from the current standardized liquidity trajectory to the closest prior trajectory.

**Unit:** dimensionless.

**Use:** small values indicate that the current path has a close historical analogue; large values indicate structural novelty.

### 10.2 `historical_path_percentile`

The empirical percentile of the nearest-match distance against retained historical path distances.

**Range:** `[0,1]`.

**Use:** makes path novelty interpretable within the symbol's own history.

### 10.3 `historical_match_from`

Timestamp of the closest prior non-overlapping trajectory.

**Use:** allows downstream systems to inspect what followed the historical analogue without the signal asserting that the same outcome will recur.

The signal MUST NOT emit a regime label.

---

## 11. Full-Book Morphology

Full-book morphology is optional and requires a consistent multi-level snapshot.

Its purpose is to describe how displayed liquidity is arranged after removing nominal price and size scale.

This is the portion of the liquidity signal intentionally designed for broad cross-symbol comparison.

Let:

$$
m=\frac{a+b}{2}
$$

$$
s=a-b
$$

For each book level \(i\):

$$
\boxed{r_i=\frac{|p_i-m|}{s}}
$$

This is distance from midpoint measured in units of the current spread.

For each side, let level notional be:

$$
n_i=p_iq_i
$$

and normalized weight:

$$
\boxed{w_i=\frac{n_i}{\sum_jn_j}}
$$

so:

$$
\sum_iw_i=1
$$

All morphology metrics below are independent of absolute book notional.

---

### 11.1 `depth_concentration:{bid,ask}`

$$
\boxed{H=\sum_iw_i^2}
$$

**Range:** \([1/n,1]\).

**Meaning:** concentration of displayed side notional into relatively few levels.

**Why:** Herfindahl concentration has a direct probability-mass interpretation and requires no arbitrary bins.

**Downstream use:** compare whether books distribute depth broadly or concentrate it into a few levels.

---

### 11.2 `effective_depth_levels:{bid,ask}`

$$
\boxed{N_{\mathrm{levels,eff}}=\frac{1}{H}}
$$

**Meaning:** effective number of equally weighted levels represented by the observed side.

**Downstream use:** intuitive companion to concentration.

The visible level count MUST be preserved as provenance.

---

### 11.3 `depth_entropy:{bid,ask}`

For \(n>1\):

$$
\boxed{E=-\frac{\sum_iw_i\log w_i}{\log n}}
$$

**Range:** `[0,1]`.

**Meaning:** evenness of notional distribution across visible levels.

**Use:** distinguishes concentrated from broadly distributed book shapes.

---

### 11.4 `depth_center_of_mass:{bid,ask}`

$$
\boxed{C=\sum_iw_ir_i}
$$

**Unit:** spread units.

**Meaning:** average distance from midpoint at which visible notional is concentrated.

**Downstream use:** compare near-touch versus farther-out placement independent of price level.

---

### 11.5 `depth_dispersion:{bid,ask}`

$$
\boxed{V=\sqrt{\sum_iw_i(r_i-C)^2}}
$$

**Unit:** spread units.

**Meaning:** spatial dispersion of side depth around its center of mass.

---

### 11.6 `bid_ask_shape_distance`

Treat bid and ask as weighted empirical distributions over mirrored relative distance \(r\).

Define their 1-Wasserstein distance:

$$
\boxed{W_1(P_b,P_a)}
$$

**Unit:** spread units.

**Meaning:** transport distance required to transform the mirrored bid-depth distribution into the ask-depth distribution.

**Why:** compares continuous weighted shapes without arbitrary histogram bins.

**Downstream use:** quantify structural asymmetry between the two sides.

---

### 11.7 `price_spacing_cv:{bid,ask}`

Let adjacent level gaps in spread units be:

$$
g_i=|r_{i+1}-r_i|
$$

Then:

$$
\boxed{CV_g=\frac{\operatorname{sd}(g)}{\operatorname{mean}(g)}}
$$

**Unit:** dimensionless.

**Meaning:** regularity of price-level spacing.

Low values indicate highly regular spacing; high values indicate irregular spacing.

Venue tick size MUST be preserved because exchange price discretization can contribute to this metric.

---

### 11.8 `quantity_repetition:{bid,ask}`

When the venue exposes a natural lot-size unit, quantities SHOULD first be expressed in integer lot units.

Let \(n\) be visible levels and \(u\) the number of distinct lot-normalized quantities:

$$
\boxed{R_q=1-\frac{u}{n}}
$$

**Range:** `[0,1)`.

**Meaning:** exact repetition of displayed level sizes after normalization by the venue's own quantity increment.

**Use:** structural comparison only.

**Non-claim:** repeated sizing does not establish automated or synthetic activity.

If no principled lot-size normalization exists, this metric is undefined.

---

### 11.9 `shape_change_distance:{bid,ask}`

Let \(P_t\) and \(P_{t-1}\) be successive normalized side-depth distributions over \(r\).

$$
\boxed{\Delta W_t=W_1(P_t,P_{t-1})}
$$

**Unit:** spread units.

**Meaning:** amount by which normalized book shape changed between observations.

**Downstream use:** measure morphological persistence or rapid reconstruction.

---

## 12. Cross-Symbol Comparability

The following MUST NOT be compared across arbitrary symbols as if they shared a common economic scale:

* absolute touch notional;
* absolute quantity;
* absolute spread;
* historical depth baseline.

The following MAY be compared broadly because scale is removed by construction:

* `relative_spread`;
* `touch_notional_imbalance`;
* `depth_concentration`;
* `effective_depth_levels`, with visible-level provenance;
* `depth_entropy`;
* `depth_center_of_mass`;
* `depth_dispersion`;
* `bid_ask_shape_distance`;
* `price_spacing_cv`;
* `quantity_repetition`, when normalized by venue lot size;
* `shape_change_distance`.

Cross-sectional clustering or classification of these metrics belongs downstream.

The liquidity signal does not emit `organic_book` or `synthetic_book`.

---

## 13. Cross-Signal Relationships

### 13.1 Aggressive flow / CVD

For aggressive buy notional \(F_b\):

$$
\boxed{\frac{F_b}{D_a}}
$$

compares executed buy flow with displayed ask capacity.

For aggressive sell notional \(F_s\):

$$
\boxed{\frac{F_s}{D_b}}
$$

compares executed sell flow with displayed bid capacity.

Large flow has different mechanical significance under shallow and deep displayed capacity.

### 13.2 Toxicity / cancellation accounting

Useful joint observations include:

* cancellation + falling side depth;
* cancellation + full replenishment;
* fills + falling side depth;
* retreat + stable aggregate depth.

Liquidity supplies the resulting displayed state. A toxicity signal supplies the mechanism of disappearance or execution.

Neither signal assigns intent.

### 13.3 Deeper-book flow / geometry

Compare:

* touch imbalance with full-depth imbalance;
* touch-depth divergence with whole-book notional divergence;
* touch morphology with deeper-book shape.

Disagreement between resolutions is itself a measurable relationship.

No `spoof` or `loaded` conclusion is produced here.

### 13.4 Price movement / sentiment / correlation

A price move can be contextualized by:

* current touch depth;
* depth divergence;
* spread divergence;
* liquidity SNR;
* book-shape novelty.

The same return under ordinary liquidity and highly unusual liquidity is mechanically different, even though the liquidity signal does not decide what that implies.

### 13.5 Hawkes / arrival intensity

Arrival intensity can be related to displayed capacity:

$$
\frac{\lambda_{\text{buy}}}{D_a}
\qquad
\frac{\lambda_{\text{sell}}}{D_b}
$$

or analogous rate-normalized quantities.

This combines activity tempo with available displayed liquidity.

### 13.6 Volume-rate signals

Executed volume rate can be compared with:

* depth ratio;
* depth divergence velocity;
* spread divergence;
* shape-change distance.

The liquidity signal supplies state and geometry; the volume signal supplies execution activity.

---

## 14. Invalid and Missing States

The signal MUST distinguish:

* an explicitly observed zero resting quantity;
* a missing side;
* an invalid or crossed snapshot;
* an unavailable historical baseline;
* an unavailable noise model;
* an unavailable full-book snapshot.

Rules:

1. Invalid touch geometry does not produce fabricated zero metrics.
2. Baseline-dependent metrics are undefined until a baseline exists.
3. Z-scores and SNR are undefined until noise is estimable.
4. Morphology metrics are undefined when full-book support is insufficient.
5. Historical recurrence metrics are undefined until at least one non-overlapping historical trajectory of equivalent support exists.

---

## 15. Explicit Non-Claims

The liquidity signal does not determine:

* whether liquidity is sufficient for a strategy;
* whether a market is illiquid;
* whether a book is organic or synthetic;
* whether an order is spoofing;
* whether depth represents genuine intent;
* whether a price level is support or resistance;
* whether a divergence is bullish or bearish;
* whether a recurring trajectory will produce the same future outcome;
* whether an unusual book should be traded.

Those are downstream reasoning tasks.

---

## 16. Minimal Required Metric Set

A valid touch-liquidity implementation SHOULD minimally publish:

* `best_bid_price`;
* `best_ask_price`;
* `touch_quantity:bid`;
* `touch_quantity:ask`;
* `touch_notional:bid`;
* `touch_notional:ask`;
* `midpoint`;
* `spread`;
* `relative_spread`;
* `touch_notional_imbalance`;
* `touch_notional_baseline:bid`;
* `touch_notional_baseline:ask`;
* `depth_ratio:bid`;
* `depth_ratio:ask`;
* `depth_divergence:bid`;
* `depth_divergence:ask`;
* `depth_zscore:bid`;
* `depth_zscore:ask`;
* `relative_spread_baseline`;
* `spread_divergence`;
* `spread_zscore`;
* `divergence_velocity:bid`;
* `divergence_velocity:ask`;
* `spread_divergence_velocity`;
* `historical_path_distance`;
* `historical_path_percentile`;
* `From`;
* `At`;
* `Maturity`;
* `SNR`.

Full-book morphology metrics are added when the data source supports a consistent multi-level snapshot.
