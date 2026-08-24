# Hawkes / Arrival Dynamics Signal Specification

## 1. Purpose

The Hawkes signal measures the temporal arrival structure of marked market events.

For aggressive buy and sell trades, it measures:

1. empirical event counts and arrival rates;
2. conditional buy and sell arrival intensities;
3. background arrival rates;
4. self-excitation and cross-excitation;
5. excitation decay timescales;
6. branching structure and expected propagation;
7. model fit relative to simpler arrival processes;
8. point-process innovations, SNR, maturity, and historical recurrence.

The signal measures event timing and mark interaction. It does not classify the market as explosive, igniting, trending, reflexive, bullish, bearish, or forecast-ready.

---

## 2. First Principles

Let observed trade arrivals be:

\[
\{(t_i,m_i)\}_{i=1}^N
\]

with mark:

\[
m_i\in\{b,s\}
\]

where:

- \(b\) = aggressive buy;
- \(s\) = aggressive sell.

A bivariate Hawkes process models each conditional arrival intensity as a background rate plus decaying contributions from prior events.

Using target/source indexing:

\[
\boxed{
\lambda_b(t)
=
\mu_b
+
\sum_{j:t_j<t}
\alpha_{b,m_j}
e^{-\beta_{b,m_j}(t-t_j)}
}
\]

\[
\boxed{
\lambda_s(t)
=
\mu_s
+
\sum_{j:t_j<t}
\alpha_{s,m_j}
e^{-\beta_{s,m_j}(t-t_j)}
}
\]

where:

- \(\mu_b,\mu_s\) are background or immigrant rates;
- \(\alpha_{xy}\) is the instantaneous jump in target intensity \(x\) caused by source event \(y\);
- \(\beta_{xy}>0\) is the exponential decay rate of that excitation.

Units:

\[
[\lambda]=[\mu]=[\alpha]=\text{events/second}
\]

\[
[\beta]=1/\text{second}
\]

The excitation timescale is:

\[
\boxed{
\tau_{xy}=\frac{1}{\beta_{xy}}
}
\]

The expected number of direct target-\(x\) descendants caused by one source-\(y\) event is the kernel integral:

\[
\boxed{
K_{xy}
=
\int_0^\infty
\alpha_{xy}e^{-\beta_{xy}u}\,du
=
\frac{\alpha_{xy}}{\beta_{xy}}
}
\]

The dimensionless matrix:

\[
\boxed{
K=
\begin{bmatrix}
K_{bb} & K_{bs}\\
K_{sb} & K_{ss}
\end{bmatrix}
}
\]

is the branching matrix.

This distinction is essential:

- \(\alpha\) measures immediate excitation amplitude;
- \(\beta\) measures decay rate;
- \(\alpha/\beta\) measures expected direct offspring.

They MUST NOT be conflated.

---

## 3. Model Parameterization

The required mathematical model is a non-negative bivariate exponential Hawkes process.

Parameters satisfy:

\[
\mu_x\ge0
\]

\[
\alpha_{xy}\ge0
\]

\[
\beta_{xy}>0
\]

A constrained common-decay model:

\[
\beta_{xy}=\beta
\]

MAY be used only when that constraint is explicit and justified by the model-selection procedure.

The signal MUST NOT silently impose arbitrary initial or permanent values for \(\mu\), \(\alpha\), or \(\beta\).

Initialization may be numerical, but published fitted parameters come from the observed event history.

---

## 4. Inputs

### 4.1 Required event inputs

| Input | Unit | Validity |
|---|---|---|
| event timestamp | time | finite, causally ordered |
| event mark | buy / sell | explicit |

The event mark identifies aggressor side.

Trade quantity and notional are not required by this signal; event size belongs to executed-flow measurement.

### 4.2 Event ordering

For each symbol:

\[
t_i\ge t_{i-1}
\]

The implementation MUST define how equal timestamps are ordered when the venue supplies multiple events at the same timestamp.

If a deterministic order is unavailable, simultaneous events MUST be treated as a batch so one event does not spuriously excite another solely because of arbitrary ingestion order.

---

## 5. Measurement Envelope

Every measurement contains:

- `From`;
- `At`;
- `Maturity`;
- `SNR`.

### 5.1 `From`

`From` is the earliest event contributing non-zero weight to the fitted model and compensator represented by the measurement.

### 5.2 `At`

`At` is the event time at which the conditional intensities are evaluated.

For an event at \(t_i\), the reported pre-arrival intensities are:

\[
\lambda_b(t_i^-),\qquad\lambda_s(t_i^-)
\]

The arriving event MUST NOT excite itself before its own intensity is evaluated.

### 5.3 Causal update ordering

For each new event:

1. evaluate pre-arrival intensity using parameters and history available before the event;
2. evaluate likelihood and compensator contribution;
3. emit the measurement;
4. incorporate the event into process state;
5. refit/update model parameters for subsequent observations.

The current event therefore cannot improve the model against which that same event is evaluated.

### 5.4 `Maturity`

For fitted event weights \(w_i\):

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

For an unweighted retained fit:

\[
N_{\mathrm{eff}}=N
\]

Maturity measures effective model support, not model correctness.

---

## 6. Event Count Metrics

For observation span:

\[
T=At-From
\]

### 6.1 `event_count`

\[
\boxed{
N=N_b+N_s
}
\]

**Unit:** count.

---

### 6.2 `event_count:buy`

\[
\boxed{
N_b=\sum_i\mathbf{1}(m_i=b)
}
\]

**Unit:** count.

---

### 6.3 `event_count:sell`

\[
\boxed{
N_s=\sum_i\mathbf{1}(m_i=s)
}
\]

**Unit:** count.

---

### 6.4 `event_fraction:buy`

For \(N>0\):

\[
\boxed{
F_b=\frac{N_b}{N}
}
\]

### 6.5 `event_fraction:sell`

\[
\boxed{
F_s=\frac{N_s}{N}
}
\]

with:

\[
F_b+F_s=1
\]

These describe mark composition only.

---

## 7. Empirical Arrival Rates

For \(T>0\):

### 7.1 `arrival_rate:buy`

\[
\boxed{
r_b=\frac{N_b}{T}
}
\]

**Unit:** events / second.

### 7.2 `arrival_rate:sell`

\[
\boxed{
r_s=\frac{N_s}{T}
}
\]

### 7.3 `arrival_rate`

\[
\boxed{
r=r_b+r_s=\frac{N}{T}
}
\]

**Meaning:** observed event frequency over the retained interval.

Empirical rate is distinct from fitted conditional intensity.

---

## 8. Conditional Intensity Metrics

### 8.1 `conditional_intensity:buy`

\[
\boxed{
\lambda_b(t^-)
}
\]

**Unit:** events / second.

**Meaning:** model-implied instantaneous buy-arrival rate immediately before the current event.

---

### 8.2 `conditional_intensity:sell`

\[
\boxed{
\lambda_s(t^-)
}
\]

---

### 8.3 `conditional_intensity`

\[
\boxed{
\lambda(t^-)=\lambda_b(t^-)+\lambda_s(t^-)
}
\]

**Unit:** events / second.

**Why:** total instantaneous arrival intensity is required for event-time diagnostics and cross-signal activity comparison.

---

## 9. Background Rate Metrics

### 9.1 `background_rate:buy`

\[
\boxed{\mu_b}
\]

**Unit:** events / second.

### 9.2 `background_rate:sell`

\[
\boxed{\mu_s}
\]

### 9.3 `background_rate`

\[
\boxed{
\mu=\mu_b+\mu_s
}
\]

**Meaning:** fitted immigrant/background event intensity in the absence of excitation from retained prior events.

The signal does not interpret background flow as informed, uninformed, organic, or noise.

---

## 10. Excess Intensity Metrics

### 10.1 `excitation_intensity:buy`

\[
\boxed{
e_b(t)=\lambda_b(t)-\mu_b
}
\]

**Unit:** events / second.

### 10.2 `excitation_intensity:sell`

\[
\boxed{
e_s(t)=\lambda_s(t)-\mu_s
}
\]

### 10.3 `excitation_fraction:buy`

For \(\lambda_b>0\):

\[
\boxed{
q_b=
\frac{\lambda_b-\mu_b}{\lambda_b}
}
\]

### 10.4 `excitation_fraction:sell`

\[
\boxed{
q_s=
\frac{\lambda_s-\mu_s}{\lambda_s}
}
\]

**Range:** `[0,1]` for the non-negative Hawkes model.

**Meaning:** fraction of current fitted intensity attributable to prior-event excitation rather than fitted background intensity.

This is a model decomposition, not a probability that the next event was "caused" by a particular previous event.

---

## 11. Excitation Amplitude Metrics

Using target/source naming:

### 11.1 `excitation_amplitude:buy_from_buy`

\[
\boxed{\alpha_{bb}}
\]

### 11.2 `excitation_amplitude:buy_from_sell`

\[
\boxed{\alpha_{bs}}
\]

### 11.3 `excitation_amplitude:sell_from_buy`

\[
\boxed{\alpha_{sb}}
\]

### 11.4 `excitation_amplitude:sell_from_sell`

\[
\boxed{\alpha_{ss}}
\]

**Unit:** events / second.

**Meaning:** instantaneous intensity jump produced by one event of the source mark.

**Downstream use:** distinguish self-excitation from cross-excitation and fast/high-amplitude from slow/persistent excitation.

---

## 12. Excitation Decay Metrics

For each target/source pair:

\[
\boxed{
\beta_{xy}>0
}
\]

**Unit:** inverse second.

Recommended metric names:

- `excitation_decay:buy_from_buy`;
- `excitation_decay:buy_from_sell`;
- `excitation_decay:sell_from_buy`;
- `excitation_decay:sell_from_sell`.

### 12.1 `excitation_timescale:*`

\[
\boxed{
\tau_{xy}=\frac{1}{\beta_{xy}}
}
\]

**Unit:** seconds.

**Meaning:** exponential e-folding time of the corresponding excitation kernel.

The timescale is often more intuitive downstream than the inverse-second decay coefficient; both MAY be published.

---

## 13. Branching Matrix Metrics

For every target/source pair:

\[
\boxed{
K_{xy}=\frac{\alpha_{xy}}{\beta_{xy}}
}
\]

Recommended names:

- `offspring:buy_from_buy`;
- `offspring:buy_from_sell`;
- `offspring:sell_from_buy`;
- `offspring:sell_from_sell`.

**Unit:** dimensionless expected events per source event.

### 13.1 Interpretation

For example:

\[
K_{bs}=0.2
\]

means that, under the fitted model, one aggressive sell event contributes an expected \(0.2\) direct aggressive buy descendants through that kernel over its entire future decay.

It does not mean that 20% of sells literally cause a buy.

---

## 14. Spectral Radius

Let:

\[
K=
\begin{bmatrix}
K_{bb}&K_{bs}\\
K_{sb}&K_{ss}
\end{bmatrix}
\]

The spectral radius is:

\[
\boxed{
\rho(K)=\max_j|\lambda_j(K)|
}
\]

For a \(2\times2\) non-negative matrix:

\[
\boxed{
\rho(K)=
\frac{
\operatorname{tr}(K)
+
\sqrt{
\operatorname{tr}(K)^2-4\det(K)
}
}{2}
}
\]

### 14.1 `branching_spectral_radius`

**Unit:** dimensionless.

**Why:** for a stationary linear Hawkes process, the mathematical stability condition is:

\[
\boxed{
\rho(K)<1
}
\]

The boundary `1` is a theorem-derived property of the model, not a tuned threshold.

The signal reports \(\rho(K)\); it does not label the market stable, unstable, explosive, or dangerous.

---

## 15. Expected Descendant Metrics

Finite expected total cluster propagation exists only when:

\[
\rho(K)<1
\]

The expected total descendants excluding the ancestor are:

\[
\boxed{
D=
K(I-K)^{-1}
}
\]

For a source mark represented by basis vector \(e_y\), expected descendants of all marks are:

\[
\boxed{
d_y=
\mathbf{1}^{\top}
K(I-K)^{-1}e_y
}
\]

Recommended metrics:

- `expected_descendants_from_buy`;
- `expected_descendants_from_sell`.

**Unit:** expected event count.

If:

\[
\rho(K)\ge1
\]

these metrics are undefined.

The implementation MUST NOT clamp, substitute, or fabricate a finite value.

---

## 16. Point-Process Likelihood

For fitted parameter vector \(\theta\), the multivariate Hawkes log likelihood over \([From,At]\) is:

\[
\boxed{
\ell_H(\theta)
=
\sum_{i=1}^{N}
\log
\lambda_{m_i}(t_i^-)
-
\sum_{x\in\{b,s\}}
\int_{From}^{At}\lambda_x(t)\,dt
}
\]

The integral term is the compensator.

### 16.1 `log_likelihood:hawkes`

\[
\boxed{\ell_H}
\]

**Unit:** natural log units (`nat`).

Raw likelihood depends on observation count and span, so per-event forms SHOULD also be published.

### 16.2 `log_likelihood_per_event:hawkes`

\[
\boxed{
\bar\ell_H=\frac{\ell_H}{N}
}
\]

for \(N>0\).

---

## 17. Poisson Baseline Model

The independent marked Poisson baseline has:

\[
\alpha_{xy}=0
\]

and constant fitted rates:

\[
\mu_b,\mu_s
\]

estimated on the same observation support.

Let:

\[
\ell_P
\]

be its log likelihood.

### 17.1 `log_likelihood:poisson`

\[
\boxed{\ell_P}
\]

### 17.2 `log_likelihood_gain_vs_poisson`

\[
\boxed{
\Delta\ell_P=\ell_H-\ell_P
}
\]

### 17.3 `log_likelihood_gain_per_event_vs_poisson`

\[
\boxed{
\frac{\ell_H-\ell_P}{N}
}
\]

**Meaning:** improvement in in-sample event-time fit per observed event relative to a constant-rate marked Poisson model.

It is not forecast validation.

---

## 18. Self-Only Hawkes Baseline

A self-only model sets:

\[
\alpha_{bs}=\alpha_{sb}=0
\]

while fitting:

\[
\mu_b,\mu_s,\alpha_{bb},\alpha_{ss},\beta_{bb},\beta_{ss}
\]

on the same support.

Let its log likelihood be:

\[
\ell_S
\]

### 18.1 `log_likelihood:self_only`

\[
\boxed{\ell_S}
\]

### 18.2 `log_likelihood_gain_vs_self_only`

\[
\boxed{
\Delta\ell_S=\ell_H-\ell_S
}
\]

### 18.3 `log_likelihood_gain_per_event_vs_self_only`

\[
\boxed{
\frac{\ell_H-\ell_S}{N}
}
\]

**Meaning:** fitted value added by cross-mark excitation beyond self-excitation.

This is a model comparison, not evidence for a market narrative.

---

## 19. Compensator and Innovation Metrics

For side \(x\), define integrated conditional intensity:

\[
\boxed{
\Lambda_x
=
\int_{From}^{At}
\lambda_x(t)\,dt
}
\]

This is the expected count under the fitted conditional process.

### 19.1 `compensator:buy`

\[
\boxed{\Lambda_b}
\]

### 19.2 `compensator:sell`

\[
\boxed{\Lambda_s}
\]

### 19.3 `count_innovation:buy`

\[
\boxed{
M_b=N_b-\Lambda_b
}
\]

### 19.4 `count_innovation:sell`

\[
\boxed{
M_s=N_s-\Lambda_s
}
\]

Under a correctly specified point process, the compensated count process is a martingale.

### 19.5 `standardized_innovation:buy`

For \(\Lambda_b>0\):

\[
\boxed{
Z_b=
\frac{N_b-\Lambda_b}
{\sqrt{\Lambda_b}}
}
\]

### 19.6 `standardized_innovation:sell`

\[
\boxed{
Z_s=
\frac{N_s-\Lambda_s}
{\sqrt{\Lambda_s}}
}
\]

**Meaning:** observed count error in units of the process's own expected counting-noise scale.

Large residuals indicate model mismatch for the observed interval, not a market category.

---

## 20. Signal-to-Noise Ratio

Hawkes SNR uses the point process's intrinsic martingale noise model.

For side \(x\), background expected count is:

\[
\boxed{
B_x=\mu_xT
}
\]

Total conditional expected count is:

\[
\Lambda_x
\]

The integrated expected contribution from excitation is:

\[
\boxed{
E_x=\Lambda_x-B_x
}
\]

For the counting-process martingale:

\[
N_x-\Lambda_x
\]

the predictable noise power over the interval is governed by:

\[
\Lambda_x
\]

Therefore the side SNR is:

\[
\boxed{
SNR_x=
\frac{E_x^2}{\Lambda_x}
}
\]

for \(\Lambda_x>0\).

The joint measurement SNR over the \(k\) defined channels is:

\[
\boxed{
SNR=
\frac{1}{k}
\sum_x
\frac{E_x^2}{\Lambda_x}
}
\]

For the bivariate buy/sell process:

\[
k\le2
\]

### 20.1 Meaning

SNR answers:

> How large is the fitted excitation contribution over the observation interval relative to the intrinsic counting noise of the conditional point process?

It does not answer:

- whether the model is correct;
- whether excitation predicts price;
- whether excitation is desirable;
- which market hypothesis is strongest.

Model correctness is evaluated separately through innovations, likelihood diagnostics, and out-of-sample validation.

---

## 21. Parameter Uncertainty

When the likelihood curvature is identifiable, the signal SHOULD estimate parameter covariance from the observed Fisher information / negative log-likelihood Hessian:

\[
\boxed{
\operatorname{Cov}(\hat\theta)
\approx
\left[
-\nabla^2\ell(\hat\theta)
\right]^{-1}
}
\]

Recommended optional metrics include standard errors for:

- background rates;
- excitation amplitudes;
- decay rates;
- offspring coefficients;
- spectral radius via delta-method propagation when appropriate.

If the Hessian is singular or numerically unreliable, parameter uncertainty is undefined rather than forced.

---

## 22. Fitting and Causality

The model MUST be fitted only on information available before the event it evaluates.

A bounded/adaptive retained history MAY be used.

Its horizon MUST be derived from observed event timing, effective support, estimator stability, or explicit model-selection criteria.

The fit MUST NOT rely on a fixed arbitrary count solely to declare readiness.

A fit is usable only when:

- parameters are finite;
- rate and decay constraints are satisfied;
- the optimizer converges under its stated criterion;
- required likelihood values are finite;
- the model is identifiable enough to produce the requested metric.

Lack of a usable fit is represented as undefined fitted metrics, not zero parameters.

---

## 23. Temporal Dynamics

Changes in fitted arrival structure MAY be measured causally.

Recommended trajectories include:

\[
\log\lambda(t)
\]

\[
\rho(K_t)
\]

\[
q_b(t),q_s(t)
\]

### 23.1 `conditional_intensity_velocity`

For positive total intensity, fit:

\[
\log\lambda_i
=
a+\gamma_\lambda(t_i-t)+\epsilon_i
\]

and publish:

\[
\boxed{
v_\lambda=\gamma_\lambda
}
\]

**Unit:** log-intensity / second.

### 23.2 `spectral_radius_velocity`

Fit:

\[
\rho_i
=
a+\gamma_\rho(t_i-t)+\epsilon_i
\]

and publish:

\[
\boxed{
v_\rho=\gamma_\rho
}
\]

**Unit:** inverse second.

Slope SNR MAY be published as:

\[
\boxed{
SNR_\gamma=
\frac{\gamma^2}
{\operatorname{Var}(\gamma)}
}
\]

Acceleration is not required.

---

## 24. Historical Recurrence

The signal MAY retain a standardized arrival-dynamics path such as:

\[
Z_t=
\begin{bmatrix}
\log\lambda_t\\
q_b(t)\\
q_s(t)\\
\rho(K_t)\\
Z_b(t)\\
Z_s(t)
\end{bmatrix}
\]

The current trajectory is compared with non-overlapping historical trajectories of equivalent causal support.

Recommended metrics:

### 24.1 `historical_path_distance`

Distance to the closest prior arrival-dynamics trajectory.

### 24.2 `historical_path_percentile`

Empirical percentile of the nearest-match distance within retained history.

### 24.3 `historical_match_from`

Start time of the nearest historical trajectory.

No regime label is emitted.

---

## 25. Relationship to CVD / Executed Flow

Hawkes measures event timing.

CVD measures event economic size and signed notional.

This distinction enables useful downstream decomposition.

Let:

\[
\lambda_b,\lambda_s
\]

be arrival intensities and:

\[
\bar n_b,\bar n_s
\]

be mean aggressive trade notionals from CVD.

Then an expected notional-arrival rate may be contextualized as:

\[
\boxed{
\lambda_b\bar n_b
}
\]

\[
\boxed{
\lambda_s\bar n_s
}
\]

**Unit:** quote currency / second.

This distinguishes:

- many small executions;
- few large executions;
- high event clustering with ordinary trade size;
- ordinary arrival intensity with unusually large trade size.

The combination belongs downstream because Hawkes itself does not observe trade size.

---

## 26. Relationship to Liquidity

Liquidity measures displayed executable capacity.

Hawkes measures the temporal density of arriving executions.

Useful downstream combinations include:

\[
\frac{\lambda_b\bar n_b}
{D_a}
\]

for aggressive buys relative to displayed ask touch notional, and:

\[
\frac{\lambda_s\bar n_s}
{D_b}
\]

for aggressive sells relative to displayed bid touch notional.

These have units:

\[
1/\text{second}
\]

and describe an expected rate of executable-touch turnover under contemporaneous event intensity and mean trade size.

The signal does not infer whether capacity will survive that flow.

---

## 27. Relationship to Toxicity

Useful combinations include:

- buy intensity + ask fill fraction;
- sell intensity + bid fill fraction;
- excitation fraction + touch replenishment rate;
- excitation fraction + touch withdrawal rate;
- standardized Hawkes innovations + unexpected liquidity retreat.

This distinguishes event clustering from the disposition of displayed liquidity.

Neither signal assigns intent.

---

## 28. Relationship to Depthflow

Depthflow measures displayed-book mutation.

Hawkes measures executed-event arrival dynamics.

Useful comparisons include:

- total conditional trade intensity vs book-turnover rate;
- buy/sell excitation fractions vs side-specific displayed-flow rates;
- cross-excitation coefficients vs changing touch/full-book imbalance;
- Hawkes innovations vs unusual book mutation.

A rapidly changing book with ordinary trade intensity differs from a rapidly changing book during strongly clustered trade arrivals.

---

## 29. Relationship to Correlation and Lead-Lag

Price correlation and lead-lag signals measure relationships in price paths.

Hawkes measures relationships between local marked arrival processes.

Downstream reasoning MAY compare:

- price-path coupling;
- event-arrival clustering;
- local buy/sell cross-excitation;
- event-time changes in those quantities.

A price correlation does not imply event-process excitation, and Hawkes excitation does not imply price causality.

---

## 30. Relationship to Derivatives

Liquidations or other derivative events SHOULD be treated as distinct marks if they are modeled by a Hawkes process.

They MUST NOT be silently folded into ordinary buy/sell trade marks.

A higher-dimensional marked Hawkes model may then measure kernels such as:

\[
K_{\text{buy},\text{liquidation}}
\]

or:

\[
K_{\text{liquidation},\text{sell}}
\]

with the same target/source semantics defined above.

---

## 31. Cross-Symbol Comparison

The following dimensionless quantities MAY be compared across symbols when the event definitions are equivalent:

- offspring coefficients \(K_{xy}\);
- branching spectral radius;
- excitation fractions;
- standardized innovations;
- per-event likelihood gains;
- SNR;
- standardized historical-path distances.

The following MUST NOT be compared across arbitrary symbols as though they shared a common scale:

- raw arrival rates;
- raw conditional intensities;
- raw background rates;
- excitation amplitudes \(\alpha\);
- decay rates \(\beta\);
- likelihood totals over unequal support.

Raw rates require same-symbol historical comparison or an explicitly comparable event population.

---

## 32. Invalid and Missing States

The signal MUST distinguish:

1. no event history;
2. insufficient fit support;
3. valid zero count for one mark;
4. zero fitted excitation;
5. unavailable fitted parameters;
6. non-identifiable parameter covariance;
7. non-stationary fitted branching structure;
8. unavailable expected descendants;
9. zero compensator;
10. invalid event ordering;
11. optimizer failure;
12. feed discontinuity.

Rules:

- a mark with zero observed events has measured count zero;
- an unfitted \(\mu\), \(\alpha\), or \(\beta\) is undefined, not zero;
- expected descendants are undefined when \(\rho(K)\ge1\);
- standardized innovations are undefined when the corresponding compensator is zero;
- SNR is undefined when its compensator/background decomposition cannot be computed;
- failed fits do not overwrite the last committed valid state unless the surrounding state contract explicitly defines a new model epoch.

---

## 33. Explicit Non-Claims

The Hawkes signal does not determine:

- whether the market is about to move;
- whether arrivals are bullish or bearish;
- whether a burst is momentum;
- whether excitation is manipulation;
- whether a fitted process is forecast-ready;
- whether a high spectral radius is dangerous;
- whether one trade literally caused another;
- whether cross-excitation implies price causality;
- whether an in-sample likelihood gain will persist out of sample;
- whether a recurring arrival pattern will produce the same future outcome.

Those are downstream reasoning or validation tasks.

---

## 34. Minimal Required Metric Set

A valid Hawkes / arrival-dynamics implementation SHOULD minimally publish:

- `event_count`;
- `event_count:buy`;
- `event_count:sell`;
- `event_fraction:buy`;
- `event_fraction:sell`;
- `arrival_rate`;
- `arrival_rate:buy`;
- `arrival_rate:sell`;
- `conditional_intensity`;
- `conditional_intensity:buy`;
- `conditional_intensity:sell`;
- `background_rate`;
- `background_rate:buy`;
- `background_rate:sell`;
- `excitation_intensity:buy`;
- `excitation_intensity:sell`;
- `excitation_fraction:buy`;
- `excitation_fraction:sell`;
- `excitation_amplitude:buy_from_buy`;
- `excitation_amplitude:buy_from_sell`;
- `excitation_amplitude:sell_from_buy`;
- `excitation_amplitude:sell_from_sell`;
- `excitation_decay:buy_from_buy`;
- `excitation_decay:buy_from_sell`;
- `excitation_decay:sell_from_buy`;
- `excitation_decay:sell_from_sell`;
- `excitation_timescale:buy_from_buy`;
- `excitation_timescale:buy_from_sell`;
- `excitation_timescale:sell_from_buy`;
- `excitation_timescale:sell_from_sell`;
- `offspring:buy_from_buy`;
- `offspring:buy_from_sell`;
- `offspring:sell_from_buy`;
- `offspring:sell_from_sell`;
- `branching_spectral_radius`;
- `expected_descendants_from_buy`;
- `expected_descendants_from_sell`;
- `log_likelihood:hawkes`;
- `log_likelihood_per_event:hawkes`;
- `log_likelihood:poisson`;
- `log_likelihood_gain_per_event_vs_poisson`;
- `log_likelihood:self_only`;
- `log_likelihood_gain_per_event_vs_self_only`;
- `compensator:buy`;
- `compensator:sell`;
- `count_innovation:buy`;
- `count_innovation:sell`;
- `standardized_innovation:buy`;
- `standardized_innovation:sell`;
- `historical_path_distance`;
- `historical_path_percentile`;
- `From`;
- `At`;
- `Maturity`;
- `SNR`.

Fitted or derived metrics whose mathematical prerequisites are not satisfied are explicitly undefined rather than replaced with zeros or fallback constants.
