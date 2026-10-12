# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791770728712199000` | **Symbol:** `ALL` | **Ticks:** `3617` | **Generated:** `2026-10-12T02:16:23Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **VALID** | declared hard domains on every observation; |z| <= rankBound(support) | 0/362 series breach a domain (0 observations); 0 z beyond rank bound |
| **0.5 Timing** | **VALID** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 10 spikes of 41306 |
| **1. Vitality** | **MEASURED** | descriptive | 27927 raw series, 27927 canonical cells, 6479 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 1621525 pairs, 22.0% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.020 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 590 emissions, H=2.635, p=0.020 |
| **5a. Ignition precursors** | **INSUFFICIENT_DATA** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5b. Exhaustion precursors** | **INSUFFICIENT_DATA** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5c. Held-out precursor skill** | **INSUFFICIENT_DATA** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 0 held-out, MCC 0.000, p=0.000 |
| **5d. Friction clearance** | **INSUFFICIENT_DATA** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **INSUFFICIENT_DATA** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 0.0% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **VALID** | cvd frames recomputed from the stored trade tape; every check exercised | 0 violations; unexercised [] |
| **V3. Causality** | **CONTRACT_BREACH** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 423574 compared; changed: future 10563, cross-symbol 1530, epoch 10426 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.010 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `362`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `0` observations beyond rank bound |z| <= rankBound(support) (max |z|=`3.00`, saturated=`0.6%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `41306`
- Ingestion latency (Timestamp - At): mean `-5.5ms`, p95 `18.7ms`, max `203.8ms`
- Latency spikes: `10`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `VALID`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `27927` (varying `24898`, constant `713`)
- Canonical grid cells: `27927` (varying `13900`, constant `6479`)
- High-correlation canonical pairs shown by the current reference filter: `42378`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|AAVE/USD|absolute_covariance_score` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|cohort_absolute_covariance_score` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|cohort_covariance_score` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|cohort_covariance_score_dispersion` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|cohort_effective_peer_count` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|cohort_peer_count` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|covariance` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|covariance_p_value` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|covariance_score` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|covariance_score_baseline` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|covariance_score_divergence` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|covariance_score_velocity` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|covariance_score_zscore` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|covariance_standard_error` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|historical_path_distance` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|historical_path_percentile` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|overlap_density` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|overlap_pair_count` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|peer_return_energy_rate` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|relative_return_energy` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|relative_return_energy_baseline` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|relative_return_energy_divergence` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|relative_return_energy_velocity` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|relative_return_energy_zscore` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|return_energy:measured` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|return_energy:reference` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|return_energy_rate:measured` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|return_energy_rate:reference` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|shared_time` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|supported_return_count:measured` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AAVE/USD|supported_return_count:reference` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ADA/USD|covariance_score` | `65.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ADA/USD|covariance_score_velocity` | `65.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ADA/USD|covariance_score_zscore` | `65.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ADA/USD|relative_return_energy_baseline` | `65.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ADA/USD|relative_return_energy_divergence` | `65.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ADA/USD|relative_return_energy_velocity` | `65.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AERO/USD|covariance_score_velocity` | `68.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AERO/USD|historical_path_distance` | `68.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AERO/USD|historical_path_percentile` | `68.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AERO/USD|relative_return_energy_velocity` | `68.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALGO/USD|absolute_covariance_score` | `72.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALGO/USD|cohort_absolute_covariance_score` | `72.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALGO/USD|cohort_covariance_score` | `72.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALGO/USD|cohort_covariance_score_dispersion` | `72.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALGO/USD|cohort_effective_peer_count` | `72.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALGO/USD|cohort_peer_count` | `72.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALGO/USD|covariance` | `72.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALGO/USD|covariance_p_value` | `72.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALGO/USD|covariance_score` | `72.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 6429 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|AAVE/USD|last_price` | `62.5%` | `90.0%` | `0.0629` | `COLD_START` |
| `correlation|AAVE/USD|observation_count` | `62.5%` | `90.0%` | `0.1890` | `COLD_START` |
| `correlation|ADA/USD|absolute_covariance_score` | `65.0%` | `92.3%` | `-0.0631` | `COLD_START` |
| `correlation|ADA/USD|cohort_absolute_covariance_score` | `65.0%` | `92.3%` | `0.1453` | `COLD_START` |
| `correlation|ADA/USD|cohort_covariance_score` | `65.0%` | `92.3%` | `0.1453` | `COLD_START` |
| `correlation|ADA/USD|cohort_covariance_score_dispersion` | `65.0%` | `92.3%` | `0.1453` | `COLD_START` |
| `correlation|ADA/USD|cohort_effective_peer_count` | `65.0%` | `92.3%` | `0.1453` | `COLD_START` |
| `correlation|ADA/USD|cohort_peer_count` | `65.0%` | `92.3%` | `0.1453` | `COLD_START` |
| `correlation|ADA/USD|covariance` | `65.0%` | `92.3%` | `0.1453` | `COLD_START` |
| `correlation|ADA/USD|covariance_p_value` | `65.0%` | `92.3%` | `0.0631` | `COLD_START` |
| `correlation|ADA/USD|covariance_score_baseline` | `65.0%` | `92.3%` | `0.1453` | `COLD_START` |
| `correlation|ADA/USD|covariance_score_divergence` | `65.0%` | `92.3%` | `0.0230` | `COLD_START` |
| `correlation|ADA/USD|covariance_standard_error` | `65.0%` | `92.3%` | `0.1453` | `COLD_START` |
| `correlation|ADA/USD|historical_path_distance` | `65.0%` | `92.3%` | `0.0230` | `COLD_START` |
| `correlation|ADA/USD|historical_path_percentile` | `65.0%` | `92.3%` | `-0.0114` | `COLD_START` |
| ... and 2490 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `1621525`
- Direct relationships: `839732`; inverse relationships: `781793`
- Signed real mean: `0.021`; signed shuffled mean: `-0.000`
- 95th percentile of `|null r|`: `0.432`
- Real `|r|` above that empirical bound: `22.0%`
- KS distance in `|r|` space: `0.222`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid reproducibility

- Early grid: `362` cells / `12` regions
- Late grid: `352` cells / `12` regions
- Shared universe: `352` cells (`97.2%`)
- Adjusted Rand Index: `1.000`

> Balanced region sizes are enforced by the partitioner and are not presented as empirical evidence. No ARI health cutoff is applied.

![Stage 3](plots/stage3_region_partitioning.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `590` across `11` regions
- Excitation strength: mean `3.819`, peak `13.379`, runner-up margin `1.368`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `23.9%`
- Real transition entropy: `2.635` bits
- Empirical dwell-block null mean: `2.827` bits
- Difference (null - real): `0.192` bits

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

- Replayed ticks: `3617`
- Tokens verified: `991`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/Users/theapemachine/Library/Caches/go-build/41/4187597f235c4197bb1b7b469ff66f99ac7d05365fc9d5f823c342f981395d7a-d/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `4147`
- Physical invariant violations: `0`
- Same-time ambiguous trades skipped: `1876`
- Comparisons per check: `map[buy_notional_rate:2191 cvd_coverage:2271 level3_replay:1035153 midpoint_log_return:2171 rate_without_elapsed_time:596 response_midpoint:at:2171 response_midpoint:from:1704 sell_notional_rate:2191 signed_net_fraction_bounded:1973 trade_book:4106 trade_fields:4147 trade_rate:2191]`
- Violations per check: `map[]`
- Unexercised checks: `[]`
- Verdict: `VALID`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `20810`
- Lookahead leakage detected: `true` (compared: `423574`, contaminated: `10563`)
- Cross-symbol contamination: `true`
- Epoch isolation passed: `false`
- Verdict: `CONTRACT_BREACH`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.040` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.028` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.054` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.018` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.004` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.000` bits (dominant: `false`)

