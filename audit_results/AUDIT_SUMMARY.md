# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791761857882169000` | **Symbol:** `ALL` | **Ticks:** `2535` | **Generated:** `2026-10-12T00:56:07Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **CONTRACT_BREACH** | declared hard domains on every observation; |z| <= sqrt(n/0.05) per stream | 0/361 series breach a domain (0 observations); 2741 z beyond bound |
| **0.5 Timing** | **VALID** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 130 spikes of 42746 |
| **1. Vitality** | **MEASURED** | descriptive | 26587 raw series, 26587 canonical cells, 895 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 2214621 pairs, 15.4% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.118 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 334 emissions, H=2.511, p=0.020 |
| **5a. Ignition precursors** | **INSUFFICIENT_DATA** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5b. Exhaustion precursors** | **INSUFFICIENT_DATA** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5c. Held-out precursor skill** | **INSUFFICIENT_DATA** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 0 held-out, MCC 0.000, p=0.000 |
| **5d. Friction clearance** | **INSUFFICIENT_DATA** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **INSUFFICIENT_DATA** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 0.0% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **CONTRACT_BREACH** | cvd frames recomputed from the stored trade tape; every check exercised | 2486 violations; unexercised [trade_touch] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 420340 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.007 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `361`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `2741` observations beyond Chebyshev bound |z| <= sqrt(N/0.05) (max |z|=`65937385.32`, saturated=`3.5%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `42746`
- Ingestion latency (Timestamp - At): mean `-22.2ms`, p95 `86.7ms`, max `396.6ms`
- Latency spikes: `130`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `VALID`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `26587` (varying `23583`, constant `553`)
- Canonical grid cells: `26587` (varying `19928`, constant `895`)
- High-correlation canonical pairs shown by the current reference filter: `42234`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|B3/USD|relative_return_energy` | `78.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|B3/USD|return_energy_rate:measured` | `78.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MINA/USD|last_price` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MINA/USD|observation_count` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|XPL/USD|covariance_score_velocity` | `52.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|XPL/USD|relative_return_energy_velocity` | `52.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|AIOZ/USD|buy_notional_rate` | `96.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|AKT/USD|sell_notional_rate` | `91.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|B3/USD|aggressive_notional:buy` | `68.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|B3/USD|buy_notional_rate` | `94.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|B3/USD|executed_quantity:buy` | `68.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|B3/USD|gross_notional_rate_zscore` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|B3/USD|signed_count_fraction` | `68.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|B3/USD|signed_net_fraction` | `68.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|B3/USD|signed_net_fraction_baseline` | `63.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|B3/USD|signed_net_fraction_divergence` | `63.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|B3/USD|trade_count:buy` | `68.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CAP/USD|historical_path_distance` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|aggressive_notional:buy` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|aggressive_notional:sell` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|cumulative_notional_delta` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|cumulative_volume_delta` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|executed_quantity:buy` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|executed_quantity:sell` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|gross_executed_quantity` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|gross_notional` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|mean_trade_notional` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|net_executed_quantity` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|net_notional` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|signed_count_fraction` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|signed_net_fraction` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|trade_count` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|trade_count:buy` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|trade_count:sell` | `40.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|aggressive_notional:buy` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|aggressive_notional:sell` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|cumulative_notional_delta` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|cumulative_volume_delta` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|executed_quantity:buy` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|executed_quantity:sell` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|gross_executed_quantity` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|gross_notional` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|mean_trade_notional` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|net_executed_quantity` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|net_notional` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|signed_count_fraction` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|signed_net_fraction` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|trade_count` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|trade_count:buy` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOT/USD|trade_count:sell` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 845 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|AIOZ/USD|cohort_covariance_score_dispersion` | `16.7%` | `80.0%` | `-0.1393` | `COLD_START` |
| `correlation|AIOZ/USD|last_price` | `96.7%` | `82.8%` | `0.3057` | `COLD_START` |
| `correlation|AIOZ/USD|relative_return_energy` | `20.0%` | `83.3%` | `-0.0503` | `COLD_START` |
| `correlation|AIOZ/USD|return_energy_rate:measured` | `20.0%` | `83.3%` | `-0.0503` | `COLD_START` |
| `correlation|AKT/USD|absolute_covariance_score` | `83.3%` | `80.0%` | `-0.2663` | `COLD_START` |
| `correlation|AKT/USD|cohort_absolute_covariance_score` | `83.3%` | `80.0%` | `-0.1822` | `COLD_START` |
| `correlation|AKT/USD|cohort_covariance_score` | `83.3%` | `80.0%` | `0.0907` | `COLD_START` |
| `correlation|AKT/USD|cohort_covariance_score_dispersion` | `66.7%` | `87.5%` | `-0.0698` | `COLD_START` |
| `correlation|AKT/USD|cohort_effective_peer_count` | `83.3%` | `80.0%` | `0.3018` | `COLD_START` |
| `correlation|AKT/USD|cohort_peer_count` | `83.3%` | `80.0%` | `0.2833` | `COLD_START` |
| `correlation|AKT/USD|covariance` | `83.3%` | `80.0%` | `-0.0853` | `COLD_START` |
| `correlation|AKT/USD|covariance_p_value` | `83.3%` | `80.0%` | `0.2723` | `COLD_START` |
| `correlation|AKT/USD|covariance_score` | `83.3%` | `80.0%` | `-0.0680` | `COLD_START` |
| `correlation|AKT/USD|covariance_score_baseline` | `83.3%` | `80.0%` | `0.1582` | `COLD_START` |
| `correlation|AKT/USD|covariance_score_divergence` | `83.3%` | `80.0%` | `0.0119` | `COLD_START` |
| ... and 1649 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `2214621`
- Direct relationships: `1101730`; inverse relationships: `1112891`
- Signed real mean: `0.015`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.494`
- Real `|r|` above that empirical bound: `15.4%`
- KS distance in `|r|` space: `0.137`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid reproducibility

- Early grid: `352` cells / `12` regions
- Late grid: `361` cells / `12` regions
- Shared universe: `352` cells (`97.5%`)
- Adjusted Rand Index: `1.000`

> Balanced region sizes are enforced by the partitioner and are not presented as empirical evidence. No ARI health cutoff is applied.

![Stage 3](plots/stage3_region_partitioning.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `334` across `11` regions
- Excitation strength: mean `23650.668`, peak `7897627.018`, runner-up margin `23649.071`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `28.1%`
- Real transition entropy: `2.511` bits
- Empirical dwell-block null mean: `2.722` bits
- Difference (null - real): `0.211` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `0` ()
- A->B Ignition: `INSUFFICIENT_DATA`, event/control `0/0`, JSD `0.000` vs null95 `0.000`
- B->C Exhaustion: `INSUFFICIENT_DATA`, event/control `0/0`, JSD `0.000` vs null95 `0.000`
- Supplemental non-excursion background observations: `0`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `0.0%` | MCC: `0.000`
- Precision / Recall: `0.0%` / `0.0%`
- Mutual Information (Predictive Gain): `0.000` bits
- Prior Base Rate: `0.0%` | Top Precursor Tokens: ``

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `0` | Round-Trip Taker Fee: `0.00` bps
- Friction Clearance Rate: `0.0%` (`0` profitable / `0` unprofitable)
- Gross Mean Return: `0.00%` | Net Mean Return after Fees: `0.00%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `0` forming `0` sequential phases (enter: `0`, exit: `0`, wait: `0`)
- Balanced accuracy: `0.0%` vs best baseline (``): `0.0%` (Raw hit rate: `0/0` `0.0%`)
- Matthews Correlation Coefficient (MCC): `0.000`
- Enter action precision / recall: `0.0%` / `0.0%`
- Label-shuffled empirical null balanced accuracy: mean `0.0%`, 95th percentile `0.0%` (empirical p-value: `0.000`)
- Separates from null: `false`
- Post-teach memory retention: `0/0` (`0.0%`)
- Trie topology: `0` nodes, max depth `0`, mean depth `0.0`, branching factor `0.00`
- Basin geometry: records `0`, span `0`, active enter basins `0`, active exit basins `0` (total: `0`)
- Decisiveness: abstention rate `0.0%`, mean confidence `0.000`, mean contrast `0.000`
- Unseen background false-alarm rate: `0.00%` spurious triggers on continuous tape

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `2535`
- Tokens verified: `446`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/var/folders/30/bdbpd0hj5wddgkxyx8pbc0mc0000gn/T/go-build4035801316/b001/exe/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `4281`
- Physical invariant violations: `2486`
- Same-time ambiguous trades skipped: `1881`
- Comparisons per check: `map[buy_notional_rate:2325 cvd_coverage:2400 level3_replay:922999 midpoint_log_return:2318 rate_without_elapsed_time:607 response_midpoint:at:2318 response_midpoint:from:1849 sell_notional_rate:2325 signed_net_fraction_bounded:2006 trade_fields:4281 trade_rate:2325]`
- Violations per check: `map[level3_replay:168 response_midpoint:at:2318]`
- Unexercised checks: `[trade_touch]`
- Verdict: `CONTRACT_BREACH`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `22810`
- Lookahead leakage detected: `false` (compared: `420340`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.018` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.048` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.007` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.050` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.009` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.013` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.004` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.001` bits (dominant: `false`)

