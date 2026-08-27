# Market Observation & Metric Allocation Specification

## 1. Status & Core Objective

**Normative architecture specification.**

The SYMM system operates under one governing objective, codified at the top of `AGENTS.md`:
> **Maximize the wallet. Minimize the time to do so.**
> A best-effort, highly principled market system. Detect real opportunity types—pumps, coils, exhaustion, liquidity vacuums, sector lifts, thin-book traps—and act on them with dynamically derived thresholds only.
> * **No shortcuts or magic numbers:** No static time horizons, hardcoded multipliers, or un-statistic denominators.
> * **No fakery or performative math:** Implement real, solid, rigorous signal math using honest market data streams.

---

## 2. The Multi-Home Principle (Zero Monoculture)

A common failure mode in market architectures is dumping every observation into a single monolithic engine (such as MCTS or a single weighted classifier) or reducing rich multi-dimensional measurements into arbitrary scalar scores.

**Invariant 1 (Domain Specialization):** Every market observation, derived metric, and envelope field MUST be routed to where its physical, statistical, or economic definition operates directly:
- Point-process arrival cascades $\to$ **Hawkes Forcing on Manifold Oscillators** & **Causal Tier 3 Active Precursors**
- Executed order flow & Kyle's price impact $\to$ **Pearl do-calculus Intervention ($do(\text{Flow})$)** & **Execution Desk Slippage Modeling**
- Book mutation & resolution gaps $\to$ **Passive Structural Intent (Causal Tier 2)** & **Anti-Spoof Bluff Vetoes**
- Quote withdrawal & fill dynamics $\to$ **Liquidity Vacuum Precursors** & **Touch Retreat Parents**
- Leverage & basis geometry $\to$ **Exogenous Squeeze Cascade Triggers (Causal Tier 1)**
- Multi-asset synchronization $\to$ **Transfer Entropy Optimal Lag Priors** & **Idiosyncratic Alpha Extraction**
- Cross-sectional breadth & dispersion $\to$ **Gross Capital Scaling Governors** & **Regime Classification**

---

## 3. Measurement Envelope Allocation

Every `Measurement` envelope emitted by signal probes carries standard metadata fields that provide foundational filtering, Bayesian shrinkage, and noise calibration:

| Field | Definition | Primary Home(s) | Operational & Mathematical Function |
| :--- | :--- | :--- | :--- |
| **`From` & `At`** | Interval $[From, At]$, Duration $\Delta t = At - From$, Age $t_{\text{now}} - At$ | **Relation Layer**, **Temporal Governor**, **Causal DAG** | Enforces causal time ordering ($t_i \le At$); $\Delta t$ derives rate denominators (events/s, quote/s); age filters stale ticks. |
| **`PeerAt` & `PeerFrom`** | Asynchronous peer coordinates for bivariate/cross-asset signals | **LeadLag**, **Hayashi-Yoshida Correlation** | Enables continuous-time asynchronous interval alignment without synthetic zero-order hold ticks. |
| **`Maturity`** | $M = 1 - \frac{1}{N_{\text{eff}}}$ where $N_{\text{eff}} = \frac{(\sum w_i)^2}{\sum w_i^2}$ | **Causal Ladder**, **Category Evidence**, **MCTS** | Dynamic Bayesian Shrinkage factor: scales estimator confidence toward prior when support is sparse ($M \to 0$). Gates unready models. |
| **`SNR`** | Scalar: $z_t^2 = \frac{\delta_t^2}{\sigma_{t-}^2}$<br>Vector: $\frac{1}{k} \delta_t^\top \Sigma_{t-}^{-1} \delta_t$ | **Resonance Engine**, **Influence Graph**, **Risk Sizing** | Departure power relative to empirical noise floor $\sigma_{t-}^2$; scales causal backdoor weights and sizes dynamic entry noise bands. |
| **`SNRDefined`** | Boolean distinguishing true zero departure ($\delta=0$) from undefined noise covariance | **Causal Engine**, **MCTS Identification** | Zero-tolerance invariant: prevents treating unestimable noise models as zero departure. |

---

## 4. Signal-to-Home Allocation Matrix

### 4.1 Hawkes / Arrival Dynamics (`signal/hawkes`)
- **`conditional_intensity:buy` / `:sell`**: Injected directly into the **Physics Manifold** as oscillator energy forcing $E_{\text{osc}} \propto \lambda(t^-)$; acts as active precursor in **Causal Tier 3**.
- **`background_rate:buy` / `:sell`**: Exogenous un-excited arrival rate; acts as autonomous root in **Causal Tier 1**.
- **`branching_spectral_radius` ($\rho(K)$)**: Theorem-derived system stability metric; $\rho \to 1.0$ triggers `VerticalIgnition` arming in the **Opportunity Synthesizer** and `Turbulent` in the **Category Classifier**.
- **`excitation_timescale` ($\tau = 1/\beta$)**: Sets the dynamic lookback horizon in the **Temporal Governor** and the decay speed for **Trailing Stops**.
- **`expected_descendants` ($K(I-K)^{-1}$)**: Forecasts expected cascade cluster size for **First-Passage Holding Horizon** estimation.
- **`standardized_innovation` ($Z_b, Z_s$)**: Martingale surprise metric fed to **Resonance Engine** for model error tracking.

### 4.2 CVD / Executed Flow (`signal/cvd`)
- **`signed_net_fraction` ($\phi$) & `zscore`**: Primary active execution treatment in **Pearl Causal Ladder** ($do(\text{Flow})$); evidence for `AggressiveDrive` in **Category Classifier**.
- **`gross_notional_rate` & `zscore`**: Turnover velocity in **Causal Tier 3**; high gross rate with flat price return identifies iceberg absorption.
- **`midpoint_response_per_net_notional`**: Kyle's lambda ($\lambda_{\text{Kyle}}$); calibrates dynamic market depth elasticity and slippage in the **Broker Desk** and **Risk Plan**.
- **`midpoint_log_return` & `flow_aligned_midpoint_return`**: Ground-truth price outcome node in **Causal Tier 4** and **Resonance Ledgers**.
- **`gross_notional_rate_velocity`**: Deceleration of trading volume under peak price triggers `Exhaustion` in the **Opportunity Synthesizer**.

### 4.3 Depthflow / Order Book Mutation (`signal/depthflow`)
- **`book_imbalance` & `zscore`**: Full visible book asymmetry; passive resting intent pulling price in **Causal Tier 2**.
- **`touch_imbalance` & `zscore`**: Immediate top-of-book pressure for sub-second limit order pricing in the **Broker Desk**.
- **`imbalance_resolution_gap` ($G = I^{\text{touch}} - I^{\text{book}}$)**: Anti-spoof metric; unmasks bluff walls and issues a hard entry veto in **Causal & Allocation Stages**.
- **`book_turnover_rate`**: Book mutation frequency; controls Planck relaxation and dissipation rates in the **Physics Manifold**.

### 4.4 Toxicity / Liquidity Disposition (`signal/toxicity`)
- **`withdrawal_fraction` (`:bid`, `:ask`) & `zscore`**: Quote pulling frequency near touch; precursor for `ToxicBluff` in **Category Classifier**.
- **`fill_fraction` (`:bid`, `:ask`) & `zscore`**: Fill rate under flow; detects `LiquidityVacuum` in **Causal Tier 3**.
- **`retreat_rate` (`:bid`, `:ask`)**: Speed of touch price retreat under pressure; direct causal parent of price return in **Causal Tier 4**.

### 4.5 Derivatives / Leverage & Basis (`signal/derivatives`)
- **`open_interest_growth_rate` & `zscore`**: Leverage acceleration; confirms new capital entry in `LeveragedIgnition`.
- **`basis` & `basis_zscore`**: Perp vs spot premium/discount; leading price discovery indicator in **Causal Tier 1**.
- **`liquidation_notional_rate` & `signed_fraction`**: Involuntary forced flow rate and skew; activates `ShortSqueeze` archetypes.

### 4.6 Liquidity / Capacity & Spread (`signal/liquidity`)
- **`relative_spread`**: Market friction; sets kinematic viscosity $\nu$ in the **Physics Manifold** and defines `NoiseBand` / `MinEdge` in **Risk Plans**.
- **`two_sided_touch_notional`**: Available executable capacity; caps maximum position size in **Allocation Policy**.

### 4.7 PumpDump / Volume-Clocked Dynamics (`signal/pumpdump`)
- **`volume_bar_quantity` & `volume_rate`**: Volume lift (RVOL); anchor for `VerticalIgnition` in the **Opportunity Synthesizer**.
- **`relative_spread` & `spread_zscore`**: Compression before expansion; detects `CoiledCompression`.

### 4.8 LeadLag / Temporal Synchronization (`signal/leadlag`)
- **`best_lag_seconds` & `lag_zscore`**: Optimal time shift; supplies prior lag to the **Relation Layer**.
- **`best_lag_correlation` & `correlation_gain`**: Direct predictor of follower return in **Causal Tier 1 $\to$ Tier 4**.

### 4.9 Sentiment / Market Breadth (`signal/sentiment`)
- **`directional_consensus` & `agreement`**: Macro unanimity; prevents counter-trend shorting during universal risk-on tides.
- **`breadth_zscore` & `advance_fraction`**: Scales portfolio gross capital utilization in the **Portfolio Governor**.

### 4.10 Correlation / Multilateral Coupling (`signal/correlation`)
- **`signed_correlation` & `cohort_correlation`**: Beta coupling; distinguishes macro tide from idiosyncratic alpha.
- **`relative_return_energy_zscore`**: Idiosyncratic price energy; prioritizes high-energy independent movers in **Allocation**.
- **`correlation_divergence` & `dispersion`**: Cross-pair stress spike; triggers Pearl Causal Panic mode (inverting roles to liquidity-driven).

### 4.11 Exhaust / Microstructure Support-State (`signal/exhaust`)
- **`depth_ask_divergence_velocity`**: Wall collapse speed; triggers emergency profit-locking in **Trailing Stop Geometry**.
- **`total_depth_zscore`**: Structural rot; reduces holding horizon in **First-Passage Survival Solvers**.

---

## 5. High-Value Cross-Signal Conjunctions

1. **Active Touch Depletion Rate ($1/\text{s}$)**:
   $$\text{Depletion Rate} = \frac{\lambda_{\text{buy}} \cdot \bar{n}_{\text{buy}}}{D_{\text{ask}}}$$
   Relates Hawkes arrival tempo ($\lambda$), CVD trade size ($\bar{n}$), and displayed ask capacity ($D$).

2. **Anti-Spoof Bluff Index**:
   $$\text{Bluff Index} = (I^{\text{touch}} - I^{\text{book}}) \times \text{Withdrawal Fraction}_{\text{bid}}$$
   Unmasks fake resting support walls and triggers an immediate entry veto.

3. **Multi-Leg Ignition Conjunction (Geometric Mean)**:
   $$\text{Ignition Evidence} = \left( Z_{\text{volume\_rate}} \cdot Z_{\text{CVD\_net}} \cdot \rho(K)_{\text{Hawkes}} \cdot Z_{\text{OI\_growth}} \cdot (1 - \text{Spread}_{\text{rel}}) \right)^{1/5}$$
   Requires unanimous cross-signal confirmation across volume clock, executed flow, arrival cascade, derivative leverage, and spread compression.

---

## 6. Telemetry & Hindsight Integration

All evaluated decisions record full precursor telemetry into `decision.Alternatives` (including Hawkes supercriticality, ask retreat velocity, basis tension, CVD signed/gross flow, Depthflow book imbalance & resolution gap, Liquidity spread, Lead-Lag correlation, Sentiment consensus, Derivatives OI growth & liquidation rate, and Toxicity withdrawal/fill fractions).

The **Hindsight Workflow** (`backtest/hindsight/`) decodes these alternatives to diagnose exact root causes for missed or blocked opportunities without loss of microstructural context.
