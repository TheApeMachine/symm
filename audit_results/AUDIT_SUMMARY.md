# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791672795036580000` | **Symbol:** `ALL` | **Ticks:** `39636` | **Generated:** `2026-10-11T02:19:40Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **CONTRACT_BREACH** | declared hard domains on every observation; |z| <= sqrt(n/0.05) per stream | 0/362 series breach a domain (0 observations); 3397 z beyond bound |
| **0.5 Timing** | **CONTRACT_BREACH** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 37624 spikes of 636862 |
| **1. Vitality** | **MEASURED** | descriptive | 94920 raw series, 94920 canonical cells, 2721 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 10644477 pairs, 14.3% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.012 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 24698 emissions, H=2.768, p=0.020 |
| **5a. Ignition precursors** | **NOT_SUPPORTED** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.007 bits, p=0.745 |
| **5b. Exhaustion precursors** | **NOT_SUPPORTED** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.009 bits, p=0.235 |
| **5c. Held-out precursor skill** | **SUPPORTED** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 23720 held-out, MCC 0.020, p=0.039 |
| **5d. Friction clearance** | **NOT_A_TEST** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **NOT_SUPPORTED** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 33.0% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **VALID** | cvd frames recomputed from the stored trade tape; every check exercised | 0 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 7354255 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.001 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `362`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `3397` observations beyond Chebyshev bound |z| <= sqrt(N/0.05) (max |z|=`66725493.20`, saturated=`2.9%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `636862`
- Ingestion latency (Timestamp - At): mean `31.0ms`, p95 `232.5ms`, max `11694.0ms`
- Latency spikes: `37624`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `CONTRACT_BREACH`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `94920` (varying `88889`, constant `1661`)
- Canonical grid cells: `94920` (varying `80950`, constant `2721`)
- High-correlation canonical pairs shown by the current reference filter: `96780`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|0G/USD|relative_return_energy` | `93.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|0G/USD|return_energy_rate:measured` | `93.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AB/USD|relative_return_energy` | `67.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AIOZ/USD|relative_return_energy` | `53.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALT/USD|relative_return_energy` | `57.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ALT/USD|return_energy_rate:measured` | `57.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ATH/USD|relative_return_energy` | `59.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BICO/USD|relative_return_energy_velocity` | `52.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|DYDX/USD|covariance_score_velocity` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|DYDX/USD|relative_return_energy_velocity` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EGLD/USD|relative_return_energy` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|FLOKI/USD|relative_return_energy` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|FORTH/USD|relative_return_energy` | `84.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|absolute_covariance_score` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|cohort_absolute_covariance_score` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|cohort_covariance_score` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|cohort_covariance_score_dispersion` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|cohort_effective_peer_count` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|cohort_peer_count` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|covariance` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|covariance_p_value` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|covariance_score` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|covariance_score_baseline` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|covariance_score_divergence` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|covariance_score_velocity` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|covariance_score_zscore` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|covariance_standard_error` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|historical_path_distance` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|historical_path_percentile` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|overlap_density` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|overlap_pair_count` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|peer_return_energy_rate` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|relative_return_energy` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|relative_return_energy_baseline` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|relative_return_energy_divergence` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|relative_return_energy_velocity` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|relative_return_energy_zscore` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|return_energy:measured` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|return_energy:reference` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|return_energy_rate:measured` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|return_energy_rate:reference` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|shared_time` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|supported_return_count:measured` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GMX/USD|supported_return_count:reference` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|IAG/USD|relative_return_energy` | `76.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|KEEP/USD|covariance_score_velocity` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|KEEP/USD|relative_return_energy_velocity` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|NMR/USD|covariance_score_velocity` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|NMR/USD|relative_return_energy_velocity` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|OMI/USD|relative_return_energy` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 2671 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|A/USD|absolute_covariance_score` | `66.7%` | `83.3%` | `-0.1270` | `COLD_START` |
| `correlation|A/USD|cohort_absolute_covariance_score` | `66.7%` | `83.3%` | `0.6343` | `COLD_START` |
| `correlation|A/USD|cohort_covariance_score` | `66.7%` | `83.3%` | `0.3886` | `COLD_START` |
| `correlation|A/USD|cohort_covariance_score_dispersion` | `66.7%` | `83.3%` | `0.6190` | `COLD_START` |
| `correlation|A/USD|cohort_effective_peer_count` | `66.7%` | `83.3%` | `0.2401` | `COLD_START` |
| `correlation|A/USD|cohort_peer_count` | `66.7%` | `83.3%` | `0.1705` | `COLD_START` |
| `correlation|A/USD|covariance` | `66.7%` | `83.3%` | `0.0327` | `COLD_START` |
| `correlation|A/USD|covariance_p_value` | `66.7%` | `83.3%` | `0.1264` | `COLD_START` |
| `correlation|A/USD|covariance_score` | `66.7%` | `83.3%` | `-0.1544` | `COLD_START` |
| `correlation|A/USD|covariance_score_baseline` | `66.7%` | `83.3%` | `0.3997` | `COLD_START` |
| `correlation|A/USD|covariance_score_divergence` | `66.7%` | `83.3%` | `0.1947` | `COLD_START` |
| `correlation|A/USD|covariance_score_zscore` | `66.7%` | `83.3%` | `0.0712` | `COLD_START` |
| `correlation|A/USD|covariance_standard_error` | `66.7%` | `83.3%` | `-0.0979` | `COLD_START` |
| `correlation|A/USD|historical_path_distance` | `66.7%` | `83.3%` | `-0.0107` | `COLD_START` |
| `correlation|A/USD|historical_path_percentile` | `66.7%` | `83.3%` | `0.1058` | `COLD_START` |
| ... and 2912 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `10644477`
- Direct relationships: `5409210`; inverse relationships: `5235267`
- Signed real mean: `0.017`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.412`
- Real `|r|` above that empirical bound: `14.3%`
- KS distance in `|r|` space: `0.139`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid reproducibility

- Early grid: `362` cells / `12` regions
- Late grid: `362` cells / `12` regions
- Shared universe: `362` cells (`100.0%`)
- Adjusted Rand Index: `1.000`

> Balanced region sizes are enforced by the partitioner and are not presented as empirical evidence. No ARI health cutoff is applied.

![Stage 3](plots/stage3_region_partitioning.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `24698` across `12` regions
- Excitation strength: mean `4171.790`, peak `20848327.197`, runner-up margin `4145.017`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `36.1%`
- Real transition entropy: `2.768` bits
- Empirical dwell-block null mean: `2.922` bits
- Difference (null - real): `0.155` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `4971` (chop, down, flat, up, up_friction)
- A->B Ignition: `NOT_SUPPORTED`, event/control `5029/49876`, JSD `0.007` vs null95 `0.018`
- B->C Exhaustion: `NOT_SUPPORTED`, event/control `2599/2237`, JSD `0.009` vs null95 `0.013`
- Supplemental non-excursion background observations: `14485`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `51.6%` | MCC: `0.020`
- Precision / Recall: `10.3%` / `58.9%`
- Mutual Information (Predictive Gain): `0.000` bits
- Prior Base Rate: `9.7%` | Top Precursor Tokens: `R01, R02, R03, R06, R08, R09, R10, R12`

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `804` | Round-Trip Taker Fee: `160.00` bps
- Friction Clearance Rate: `14.8%` (`119` profitable / `685` unprofitable)
- Gross Mean Return: `0.96%` | Net Mean Return after Fees: `-0.64%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `4971` forming `4867` sequential phases (enter: `108`, exit: `112`, wait: `4647`)
- Balanced accuracy: `33.0%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `3862/4867` `79.4%`)
- Matthews Correlation Coefficient (MCC): `0.091`
- Enter action precision / recall: `21.7%` / `4.6%`
- Label-shuffled empirical null balanced accuracy: mean `32.9%`, 95th percentile `35.4%` (empirical p-value: `0.471`)
- Separates from null: `false`
- Post-teach memory retention: `3560/4867` (`73.1%`)
- Trie topology: `28679` nodes, max depth `717`, mean depth `63.8`, branching factor `1.06`
- Basin geometry: records `28678`, span `717`, active enter basins `72`, active exit basins `103` (total: `175`)
- Decisiveness: abstention rate `82.9%`, mean confidence `0.017`, mean contrast `22.281`
- Unseen background false-alarm rate: `6.81%` spurious triggers on continuous tape

> ⚠️ **LEARNING DEFICIT:** Prequential balanced accuracy (33.0%) trails baseline policy (33.3%). Memory retention is at 73.1%.

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `39636`
- Tokens verified: `56931`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/var/folders/30/bdbpd0hj5wddgkxyx8pbc0mc0000gn/T/go-build2496668764/b001/exe/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `90057`
- Physical invariant violations: `0`
- Same-time ambiguous trades skipped: `41495`
- Comparisons per check: `map[buy_notional_rate:48194 cvd_coverage:48562 midpoint_log_return:48346 rate_without_elapsed_time:12904 response_midpoint:at:48346 response_midpoint:from:48346 sell_notional_rate:48194 signed_net_fraction_bounded:43682 trade_fields:90057 trade_rate:48194]`
- Violations per check: `map[]`
- Unexercised checks: `[]`
- Verdict: `VALID`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `329677`
- Lookahead leakage detected: `false` (compared: `7354255`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.048` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.041` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.002` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.045` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.019` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.000` bits (dominant: `false`)

