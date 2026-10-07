# SYMM Pipeline Health Audit

**Status:** ⚠️ ATTENTION REQUIRED | **Epoch:** `1791386907328978000` | **Symbol:** `BTC/USD` | **Ticks:** `500` | **Generated:** `2026-10-07T17:31:25Z`

---

## Executive Summary

This audit tests whether market representation retains genuine statistical structure across five operational boundaries, comparing real measurements against production data flows and empirical nulls.

| Stage | Question Tested | Status | Key Metric |
| :--- | :--- | :---: | :--- |
| **0. Contract Integrity** | Do metrics obey declared mathematical domains? | 🚨 CRITICAL | 707/6862 breaching metrics (41943 breach events) |
| **1. Metric Vitality** | Are sensors alive and non-redundant? | ✅ PASS | 4685/6862 raw healthy; 314/378 canonical cells healthy (483 redundant pairs) |
| **2. Sympathy Null** | Do deformations move together beyond noise? | ✅ PASS | Real mean 0.012 vs Null 0.000 (KS: 0.242, Sep: 54.1%) |
| **3. Grid Stability** | Does partitioning repeat across time? | ❌ FAIL | ARI: 0.062 (Overlap: 99.7% across 377 shared cells) |
| **4. Token Dynamics** | Does region compression preserve structure? | ❌ FAIL | Out-of-sample: Max Dominance: 30.1%, Entropy Reduction: 0.050 bits |
| **5. Precursor Separation** | Are patterns before B/C distinguishable? | ℹ️ INSUFFICIENT | Precursor: 5 excursions (chop/down/flat/up/up_friction). Hypothesis A->B (Ignition): INSUFFICIENT_DATA (JSD=0.000 bits vs Null-95=0.000, N=0). Hypothesis B->C (Exhaustion): INSUFFICIENT_DATA (JSD=0.000 bits, N=378). Background control = 0 tokens. |

---

### Stage 0: Metric Contract Integrity

> **The Plain-English Question:** *Are sensors strictly adhering to their declared mathematical bounds (e.g. correlations in [-1, 1], variances >= 0), or is the pipeline ingesting mathematically ungrounded numbers?*

- **Total Metrics Audited:** `6862`
- **Metrics Violating Declared Domain:** `707` (`41943` total breach observations)

#### Top Mathematical Contract Breaches:

| Metric | Declared Domain | Observed Range | Mean | Breaches / Samples |
| :--- | :---: | :---: | :---: | :---: |
| `correlation_zscore@PI/USD` | `[-1, 1]` | `[2.092, 6.203]` | `3.134` | 118 / 118 (100.0%) |
| `correlation_zscore@W/USD` | `[-1, 1]` | `[1.959, 4.235]` | `2.577` | 118 / 118 (100.0%) |
| `best_lag_correlation_zscore@W/USD` | `[-1, 1]` | `[1.514, 4.074]` | `2.025` | 116 / 116 (100.0%) |
| `correlation_zscore@FLOW/USD` | `[-1, 1]` | `[1.164, 2.483]` | `1.540` | 118 / 118 (100.0%) |
| `best_lag_correlation_zscore@MOG/USD` | `[-1, 1]` | `[-1.144, 2.368]` | `1.784` | 71 / 71 (100.0%) |
| `contemporaneous_correlation@NOT/USD` | `[-1, 1]` | `[1.225, 2.286]` | `1.487` | 116 / 116 (100.0%) |
| `best_lag_correlation@NOT/USD` | `[-1, 1]` | `[1.225, 2.286]` | `1.487` | 116 / 116 (100.0%) |
| `absolute_correlation@NOT/USD` | `[0, 1]` | `[1.225, 2.038]` | `1.467` | 118 / 118 (100.0%) |
| `signed_correlation@NOT/USD` | `[-1, 1]` | `[1.225, 2.038]` | `1.467` | 118 / 118 (100.0%) |
| `best_lag_correlation_zscore@BDXN/USD` | `[-1, 1]` | `[-1.866, 1.553]` | `-0.746` | 65 / 65 (100.0%) |

> [!IMPORTANT]
> **Mathematical Root Cause Analysis:**
> CRITICAL CONTRACT BREACH: 707 metrics emitted values outside mathematical bounds.
> Top breaches include Hayashi-Yoshida correlation estimators:
> - correlation_zscore@PI/USD (max=6.203, mean=3.134, breaches=118/118)
> - correlation_zscore@W/USD (max=4.235, mean=2.577, breaches=118/118)
> - best_lag_correlation_zscore@W/USD (max=4.074, mean=2.025, breaches=116/116)
> - correlation_zscore@FLOW/USD (max=2.483, mean=1.540, breaches=118/118)
> - best_lag_correlation_zscore@MOG/USD (max=2.368, mean=1.784, breaches=71/71)
> ROOT CAUSE DIAGNOSIS: The Hayashi-Yoshida estimator in nomagique/algo/hayashi_yoshida.go computes cov / sqrt(leftEnergy * rightEnergy). In asynchronous high-frequency sampling with overlapping trade intervals, single intervals on one asset overlap multiple trade intervals of peer assets. Cauchy-Schwarz does not apply to discrete multi-overlapping intervals in finite samples without positive semi-definite (PSD) regularization, causing the unconstrained estimator to mathematically exceed 1.0. Clamping is prohibited as it conceals the mathematical violation.

![Stage 0 Metric Contracts](plots/stage0_metric_contracts.png)

### Stage 1: Metric Vitality & Redundancy

> **The Plain-English Question:** *Are the individual metrics actually moving and present, or are we feeding the grid dead constants and duplicated signals?*

#### Raw Producer Outputs:
- **Total Raw Named Series:** `6862`
- **Healthy Raw Series:** `4685` (moving, non-constant)
- **Dead / Constant Raw Series:** `1076`
- **Sporadic Raw Series:** `1101` (coverage < 20%)

#### Canonical Grid Inputs:
- **Canonical Cells Universe:** `378` (after peer aggregation via `ChannelsFrom`)
- **Healthy Canonical Cells:** `314`
- **Dead / Constant Canonical Cells:** `63`
- **Sporadic Canonical Cells:** `1`
- **Redundant Cell Pairs (|r| >= 0.95):** `483`

> [!NOTE]
> High correlation between baselines, z-scores, and raw signals is mathematically expected and confirms the grid's operational role: discovering redundancy to form lower-dimensional co-movement regions.

![Stage 1 Metric Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Metric Redundancy](plots/stage1_metric_redundancy.png)

### Stage 2: Pair Relationships & Sympathy (Real Deformations vs. Shuffled Null)

> **The Plain-English Question:** *Do scale-free deformations move together beyond noise, and does the grid recognize stable opposition as affinity?*

- **Pairs Analyzed:** `49141`
- **Positive Sympathy (r > 0.05):** `16438`
- **Inverse Sympathy (r < -0.05):** `15146` (stable opposition)
- **Separation vs Shuffled Null:** `54.1%` of pairs exceed the 95th percentile null envelope.
- **Kolmogorov-Smirnov Distance:** `0.242`

> [!NOTE]
> Production `PairStats.affinity()` defines sympathy as absolute alignment `math.Abs(corr) * reliability`, ensuring both lockstep co-movement and lockstep opposition contribute to graph edge weights without violating graph Laplacian PSD.

![Stage 2 Sympathy Null](plots/stage2_sympathy_null.png)

![Stage 2 Orientation Balance](plots/stage2_orientation_balance.png)

### Stage 3: Grid Partitioning & Temporal Stability

> **The Plain-English Question:** *Does the Impulse Map discover the same metric friendships across disjoint chronological periods, or does it redraw arbitrary clusters every time?*

- **Early Period Grid:** `20` regions across `377` cells
- **Late Period Grid:** `20` regions across `378` cells
- **Universe Overlap:** `99.7%` (`377` shared cells)
- **Adjusted Rand Index (ARI):** `0.062` (1.0 = identical partitions; 0.0 = random chance agreement)

> [!NOTE]
> Region sizes (~5% each) are algebraically enforced by `TargetRegionCount=20` and `balancedCapacities()`. The meaningful stability metric is Adjusted Rand Index (cluster membership agreement across chronological halves).

![Stage 3 Region Partitioning](plots/stage3_region_partitioning.png)

### Stage 4: Out-of-Sample Token Dynamics & Transition Structure

> **The Plain-English Question:** *Does a frozen grid emit low-entropy, structured state transitions on unseen market tape compared to a block-shuffled temporal null?*

- **Out-of-Sample Emissions:** `143` tokens across `17` unique active regions
- **Max Token Dominance:** `30.1%` (no single region monopolizes the tape)
- **Transition Entropy:** `2.133` bits vs Block-Shuffled Null `2.183` bits (Reduction: `0.050` bits)

![Stage 4 Token Dynamics](plots/stage4_token_dynamics.png)

![Stage 4 Transition Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Transition Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Precursor Informativeness (Ignition & Exhaustion)

> **The Plain-English Question:** *Does the market state before a profitable ignition (A -> B) or peak exhaustion (B -> C) exhibit statistically distinct regional signatures compared to controls and non-excursion background?*

- **Total Detections Found:** `5` (chop, down, flat, up, up_friction)
- **Hypothesis A -> B (Ignition):** `INSUFFICIENT_DATA` | Divergence: `0.000` bits vs Null-95 `0.000` bits (N=0 tokens)
- **Hypothesis B -> C (Exhaustion):** `INSUFFICIENT_DATA` | Divergence: `0.000` bits vs Null-95 `0.000` bits (N=378 tokens)
- **Disjoint Background Control:** `0` non-excursion tokens

> [!IMPORTANT]
> Precursor analysis enforces strict population isolation: background tape excludes all excursion windows. Absence of evidence or small sample sizes are reported as `INSUFFICIENT_DATA`, never `PASS`.

![Stage 5 Precursor Separation](plots/stage5_precursor_separation.png)

---

## Audit Verdict & Recommendations

### Operational Blockers Identified:

1. **Metric Contract Breaches (Stage 0):** Asynchronous Hayashi-Yoshida cross-variation estimators regularly exceed 1.0. Upstream covariance regularisation or normalization review is required before treating these sensors as bounded correlation.
2. **Grid Partition Instability (Stage 3):** Cross-period Adjusted Rand Index indicates low partition reproducibility. Sample size or edge filtering requires tuning.
3. **Precursor Evidence Insufficiency (Stage 5):** Insufficient independent excursion runs in the sampled archive to confirm ignition/exhaustion precursor separation.
