# SYMM Pipeline Health Audit

**Status:** ⚠️ ATTENTION REQUIRED | **Epoch:** `1791386907328978000` | **Symbol:** `BTC/USD` | **Ticks:** `200` | **Generated:** `2026-10-07T16:46:28Z`

---

## Executive Summary

This audit tests whether market representation retains genuine statistical structure across five operational boundaries, comparing real measurements against deliberately scrambled empirical nulls.

| Stage | Question Tested | Status | Key Metric |
| :--- | :--- | :---: | :--- |
| **1. Metric Vitality** | Are sensors alive and non-redundant? | ✅ PASS | 2624/7134 healthy, 1603 dead, 699 redundant pairs |
| **2. Sympathy Null** | Do metrics move together beyond noise? | ❌ FAIL | 6.7% exceed null envelope (KS: 0.086) |
| **3. Grid Stability** | Does partitioning repeat across time? | ❌ FAIL | ARI: 0.056 (Overlap: 100.0%, Max Share: 5.0%) |
| **4. Token Dynamics** | Does region compression preserve structure? | ✅ PASS | Max Dominance: 31.2%, Entropy Reduction: 0.535 bits |
| **5. Precursor Separation** | Are patterns before B/C distinguishable? | ✅ PASS | Precursor: 4 excursions analyzed (chop/flat/up/up_friction). Divergence = 0.343 bits vs Null-95 = 0.076 bits (Ratio: 4.49x, 50 precursor ticks). |

---

### Stage 1: Metric Vitality & Redundancy

> **The Plain-English Question:** *Are the individual metrics actually moving and present, or are we feeding the grid dead constants and duplicated signals?*

- **Healthy Metrics:** `2624` of `7134` (`36.8%`)
- **Dead / Constant Metrics:** `1603` (metrics with zero variance)
- **Sporadic Metrics:** `2907` (observed on < 20% of ticks)
- **Redundant Duplicate Pairs (|r| >= 0.95):** `699`

![Stage 1 Metric Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Metric Redundancy](plots/stage1_metric_redundancy.png)

### Stage 2: Pair Relationships & Sympathy (Real vs. Shuffled Null)

> **The Plain-English Question:** *Do metrics exhibit genuine simultaneous sympathy, or could an identical grid be built from scrambled noise?*

- **Pairs Analyzed:** `1151`
- **Positive Relationships:** `799` | **Inverse (Negative) Relationships:** `295`
- **Separation vs Shuffled Null:** `6.7%` of pairs exceed the 95th percentile null envelope.
- **Kolmogorov-Smirnov Distance:** `0.086` (higher is better; > 0.20 indicates distinct non-random distribution)

![Stage 2 Sympathy Null](plots/stage2_sympathy_null.png)

![Stage 2 Orientation Balance](plots/stage2_orientation_balance.png)

### Stage 3: Grid Partitioning & Temporal Stability

> **The Plain-English Question:** *Does the Impulse Map discover the same metric friendships across disjoint chronological periods, or does it redraw arbitrary clusters every time?*

- **Early Period Grid:** `20` regions across `377` cells (Largest region holds `5.0%`)
- **Late Period Grid:** `20` regions across `377` cells (Largest region holds `5.0%`)
- **Universe Overlap:** `100.0%` (`377` shared cells)
- **Adjusted Rand Index (ARI):** `0.056` (1.0 = identical partitions; 0.0 = random agreement; > 0.30 indicates stable clustering)

![Stage 3 Region Partitioning](plots/stage3_region_partitioning.png)

### Stage 4: Token Dynamics & State Transitions

> **The Plain-English Question:** *When metrics light regions on a frozen grid, does the resulting token stream have structured dynamics or degenerate collapse?*

- **Total Emissions:** `160` | **Active Regions Emitted:** `16`
- **Max Token Dominance:** `31.2%` (< 80% is healthy; higher indicates stuck state collapse)
- **Transition Entropy:** `1.746` bits (Null Shuffled Entropy: `2.281` bits)
- **Entropy Reduction:** `0.535` bits (higher is better; indicates organized, non-random transition dynamics)

![Stage 4 Token Dynamics](plots/stage4_token_dynamics.png)

![Stage 4 Transition Heatmap](plots/stage4_transition_matrix.png)

![Stage 4 Transition Entropy](plots/stage4_transition_entropy.png)

### Stage 5: Precursor Informativeness & Null Separation

> **The Plain-English Question:** *Are the token sequences or sensory states preceding B (ignition) statistically distinguishable from ambient market noise, or is the trie being asked to memorize noise?*

- **Detections Found:** `4` (Classes: `chop, flat, up, up_friction`)
- **Precursor Window Ticks:** `50`
- **Precursor Divergence (JSD):** `0.343` bits
- **Empirical Shuffled Null 95th Percentile:** `0.076` bits (Mean: `0.057` bits)
- **Separation Ratio:** `4.49x` (> 1.0x indicates real precursor signal exceeding 95% null envelope)

![Stage 5 Precursor Separation](plots/stage5_precursor_separation.png)

