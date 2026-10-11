# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791747550263717000` | **Symbol:** `ALL` | **Ticks:** `20536` | **Generated:** `2026-10-11T22:09:22Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **CONTRACT_BREACH** | declared hard domains on every observation; |z| <= sqrt(n/0.05) per stream | 5/362 series breach a domain (30357 observations); 4251 z beyond bound |
| **0.5 Timing** | **CONTRACT_BREACH** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 22836 spikes of 388302 |
| **1. Vitality** | **MEASURED** | descriptive | 71204 raw series, 71204 canonical cells, 3247 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 7488953 pairs, 13.8% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.000 bits, p=1.000 |
| **4. Token dynamics** | **INSUFFICIENT_DATA** | transition entropy below block null, p <= 0.05 | 0 emissions, H=0.000, p=0.000 |
| **5a. Ignition precursors** | **INSUFFICIENT_DATA** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5b. Exhaustion precursors** | **INSUFFICIENT_DATA** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5c. Held-out precursor skill** | **INSUFFICIENT_DATA** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 0 held-out, MCC 0.000, p=0.000 |
| **5d. Friction clearance** | **INSUFFICIENT_DATA** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **INSUFFICIENT_DATA** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 0.0% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **VALID** | cvd frames recomputed from the stored trade tape; every check exercised | 0 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 4499671 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **INSUFFICIENT_DATA** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.000 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `362`
- Hard-domain breaches: `5` series (`30357` observations)
- Standardized z-score bound breaches: `4251` observations beyond Chebyshev bound |z| <= sqrt(N/0.05) (max |z|=`64882165.12`, saturated=`3.1%`)
- Normalization breaches: `0`, Standardization breaches: `0`

| Metric | Unit | Declared domain | Observed range | Breaches |
| :--- | :---: | :---: | :---: | ---: |
| `leadlag:best_lag_seconds_median` | `second` | `[0, +inf)` | `[-254.602, 261.260]` | 23100/38054 |
| `sentiment:breadth_baseline` | `ratio` | `[0, +inf)` | `[-1.000, 0.370]` | 1053/2422 |
| `sentiment:breadth` | `ratio` | `[0, +inf)` | `[-1.000, 1.000]` | 835/2423 |
| `depthflow:imbalance_resolution_gap` | `ratio` | `[-1, 1]` | `[-1.969, 1.821]` | 5231/38643 |
| `depthflow:resolution_gap_baseline` | `ratio` | `[-1, 1]` | `[-1.532, 1.424]` | 138/38421 |

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `388302`
- Ingestion latency (Timestamp - At): mean `-130.3ms`, p95 `-7.3ms`, max `928.8ms`
- Latency spikes: `22836`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `CONTRACT_BREACH`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `71204` (varying `66099`, constant `1374`)
- Canonical grid cells: `71204` (varying `57320`, constant `3247`)
- High-correlation canonical pairs shown by the current reference filter: `78922`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|APE/USD|relative_return_energy` | `76.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|APE/USD|return_energy_rate:measured` | `76.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AZTEC/USD|relative_return_energy_velocity` | `56.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BICO/USD|relative_return_energy_velocity` | `76.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BILL/USD|relative_return_energy` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BILL/USD|return_energy_rate:measured` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BRL1/USD|relative_return_energy` | `93.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|CPOOL/USD|relative_return_energy_velocity` | `90.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|DAG/USD|relative_return_energy_velocity` | `90.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ETC/USD|relative_return_energy` | `94.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EUL/USD|last_price` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EUL/USD|observation_count` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EV/USD|relative_return_energy` | `96.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GIGA/USD|covariance_score_velocity` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GIGA/USD|relative_return_energy` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GIGA/USD|relative_return_energy_velocity` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|HONEY/USD|covariance_score_velocity` | `90.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|HONEY/USD|relative_return_energy_velocity` | `90.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ICNT/USD|last_price` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ICNT/USD|observation_count` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|absolute_covariance_score` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|cohort_absolute_covariance_score` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|cohort_covariance_score` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|cohort_covariance_score_dispersion` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|cohort_effective_peer_count` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|cohort_peer_count` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|covariance` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|covariance_p_value` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|covariance_score` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|covariance_score_baseline` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|covariance_score_divergence` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|covariance_score_zscore` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|covariance_standard_error` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|historical_path_distance` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|historical_path_percentile` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|overlap_density` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|overlap_pair_count` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|peer_return_energy_rate` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|relative_return_energy` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|relative_return_energy_baseline` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|relative_return_energy_divergence` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|relative_return_energy_zscore` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|return_energy:measured` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|return_energy:reference` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|return_energy_rate:measured` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|return_energy_rate:reference` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|shared_time` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|supported_return_count:measured` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|JST/USD|supported_return_count:reference` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MASK/USD|relative_return_energy` | `86.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 3197 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|ALIGN/USD|absolute_covariance_score` | `55.6%` | `80.0%` | `0.6949` | `COLD_START` |
| `correlation|ALIGN/USD|covariance_p_value` | `55.6%` | `80.0%` | `-0.6949` | `COLD_START` |
| `correlation|ALIGN/USD|covariance_score` | `55.6%` | `80.0%` | `-0.6949` | `COLD_START` |
| `correlation|ALIGN/USD|overlap_density` | `55.6%` | `80.0%` | `-0.6949` | `COLD_START` |
| `correlation|ALIGN/USD|overlap_pair_count` | `55.6%` | `80.0%` | `0.6949` | `COLD_START` |
| `correlation|ALIGN/USD|relative_return_energy` | `55.6%` | `80.0%` | `0.2459` | `COLD_START` |
| `correlation|ALIGN/USD|return_energy:reference` | `55.6%` | `80.0%` | `0.6949` | `COLD_START` |
| `correlation|ALIGN/USD|return_energy_rate:reference` | `55.6%` | `80.0%` | `0.6949` | `COLD_START` |
| `correlation|ALIGN/USD|shared_time` | `55.6%` | `80.0%` | `0.6949` | `COLD_START` |
| `correlation|ALIGN/USD|supported_return_count:reference` | `55.6%` | `80.0%` | `0.6949` | `COLD_START` |
| `correlation|APR/USD|absolute_covariance_score` | `88.9%` | `87.5%` | `0.0430` | `COLD_START` |
| `correlation|APR/USD|cohort_absolute_covariance_score` | `88.9%` | `87.5%` | `0.0992` | `COLD_START` |
| `correlation|APR/USD|cohort_covariance_score` | `88.9%` | `87.5%` | `0.1737` | `COLD_START` |
| `correlation|APR/USD|cohort_covariance_score_dispersion` | `88.9%` | `87.5%` | `0.0648` | `COLD_START` |
| `correlation|APR/USD|cohort_effective_peer_count` | `88.9%` | `87.5%` | `0.1504` | `COLD_START` |
| ... and 3299 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `7488953`
- Direct relationships: `3801065`; inverse relationships: `3687888`
- Signed real mean: `0.016`; signed shuffled mean: `-0.000`
- 95th percentile of `|null r|`: `0.422`
- Real `|r|` above that empirical bound: `13.8%`
- KS distance in `|r|` space: `0.130`

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

- Held-out emissions: `0` across `0` regions
- Excitation strength: mean `0.000`, peak `0.000`, runner-up margin `0.000`
- Active cell coverage: `0.0%` mean
- Maximum observed token share: `0.0%`
- Real transition entropy: `0.000` bits
- Empirical dwell-block null mean: `0.000` bits
- Difference (null - real): `0.000` bits

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

- Replayed ticks: `20536`
- Tokens verified: `23442`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/var/folders/30/bdbpd0hj5wddgkxyx8pbc0mc0000gn/T/go-build2222039555/b001/exe/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `38955`
- Physical invariant violations: `0`
- Same-time ambiguous trades skipped: `19636`
- Comparisons per check: `map[buy_notional_rate:19089 cvd_coverage:19319 midpoint_log_return:19158 rate_without_elapsed_time:5883 response_midpoint:at:19158 response_midpoint:from:19158 sell_notional_rate:19089 signed_net_fraction_bounded:17782 trade_fields:38955 trade_rate:19089]`
- Violations per check: `map[]`
- Unexercised checks: `[]`
- Verdict: `VALID`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `196094`
- Lookahead leakage detected: `false` (compared: `4499671`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `INSUFFICIENT_DATA`

  - Family `correlation`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.000` bits (dominant: `false`)

