# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791747550263717000` | **Symbol:** `ALL` | **Ticks:** `12009` | **Generated:** `2026-10-11T20:15:34Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **CONTRACT_BREACH** | declared hard domains on every observation; |z| <= sqrt(n/0.05) per stream | 68/362 series breach a domain (876913 observations); 3128 z beyond bound |
| **0.5 Timing** | **CONTRACT_BREACH** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 21815 spikes of 227054 |
| **1. Vitality** | **MEASURED** | descriptive | 55182 raw series, 55182 canonical cells, 2854 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 5585895 pairs, 14.0% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.008 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 5501 emissions, H=2.686, p=0.020 |
| **5a. Ignition precursors** | **INSUFFICIENT_DATA** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5b. Exhaustion precursors** | **INSUFFICIENT_DATA** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5c. Held-out precursor skill** | **INSUFFICIENT_DATA** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 0 held-out, MCC 0.000, p=0.000 |
| **5d. Friction clearance** | **INSUFFICIENT_DATA** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **INSUFFICIENT_DATA** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 0.0% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **VALID** | cvd frames recomputed from the stored trade tape; every check exercised | 0 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 2588664 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.004 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `362`
- Hard-domain breaches: `68` series (`876913` observations)
- Standardized z-score bound breaches: `3128` observations beyond Chebyshev bound |z| <= sqrt(N/0.05) (max |z|=`64882165.12`, saturated=`3.3%`)
- Normalization breaches: `0`, Standardization breaches: `0`

| Metric | Unit | Declared domain | Observed range | Breaches |
| :--- | :---: | :---: | :---: | ---: |
| `toxicity:replenishment_fraction_baseline:bid` | `ratio` | `undeclared` | `[0.000, 24860446428.573]` | 6714/6714 |
| `correlation:relative_return_energy` | `ratio` | `undeclared` | `[0.000, 27643547.744]` | 22148/22148 |
| `liquidity:depth_ratio:bid` | `ratio` | `undeclared` | `[0.000, 17143144.171]` | 22512/22512 |
| `pumpdump:notional_rate_ratio` | `ratio` | `undeclared` | `[0.000, 13852593.431]` | 10111/10111 |
| `cvd:gross_notional_rate_ratio` | `ratio` | `undeclared` | `[0.000, 5400214.988]` | 14188/14188 |
| `depthflow:turnover_ratio` | `ratio` | `undeclared` | `[0.000, 1459446.051]` | 14150/14150 |
| `toxicity:replenishment_fraction_baseline:ask` | `ratio` | `undeclared` | `[0.000, 402765.793]` | 6769/6769 |
| `correlation:relative_return_energy_baseline` | `ratio` | `undeclared` | `[0.000, 212143.191]` | 22147/22147 |
| `leadlag:last` | `dimensionless` | `undeclared` | `[0.000, 83655.000]` | 22719/22719 |
| `liquidity:depth_ratio:ask` | `ratio` | `undeclared` | `[0.000, 9128.010]` | 22512/22512 |
| `morphology:morphology_change` | `ratio` | `undeclared` | `[0.000, 1056.651]` | 22512/22512 |
| `correlation:overlap_density` | `ratio` | `undeclared` | `[0.113, 873.744]` | 22156/22156 |

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `227054`
- Ingestion latency (Timestamp - At): mean `-157.2ms`, p95 `-51.5ms`, max `928.8ms`
- Latency spikes: `21815`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `CONTRACT_BREACH`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `55182` (varying `50694`, constant `1275`)
- Canonical grid cells: `55182` (varying `43939`, constant `2854`)
- High-correlation canonical pairs shown by the current reference filter: `59622`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|AB/USD|last_price` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|AB/USD|observation_count` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|APE/USD|relative_return_energy` | `64.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|APE/USD|return_energy_rate:measured` | `64.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ARKM/USD|relative_return_energy_velocity` | `90.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ATH/USD|relative_return_energy_velocity` | `76.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BILL/USD|relative_return_energy` | `76.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BILL/USD|return_energy_rate:measured` | `76.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BIO/USD|covariance_score_velocity` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BIO/USD|relative_return_energy_velocity` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|CVX/USD|relative_return_energy` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ELIZAOS/USD|relative_return_energy_velocity` | `76.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ETC/USD|relative_return_energy` | `92.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EUL/USD|last_price` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EUL/USD|observation_count` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EV/USD|relative_return_energy` | `94.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EWT/USD|last_price` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|FOLD/USD|relative_return_energy_velocity` | `90.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MINA/USD|relative_return_energy_velocity` | `90.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|NIL/USD|relative_return_energy` | `78.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|POPCAT/USD|relative_return_energy` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|POPCAT/USD|relative_return_energy_velocity` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|POPCAT/USD|return_energy_rate:measured` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|RLC/USD|relative_return_energy_velocity` | `90.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|SKY/USD|relative_return_energy_velocity` | `83.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|WBT/USD|relative_return_energy` | `68.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|WBT/USD|relative_return_energy_velocity` | `68.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|WEMIX/USD|relative_return_energy` | `91.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|XMN/USD|relative_return_energy` | `68.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ZAMA/USD|relative_return_energy` | `95.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|AAVE/USD|sell_notional_rate` | `90.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|AKT/USD|historical_path_percentile` | `28.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|AKT/USD|sell_notional_rate` | `87.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ALTHEA/USD|sell_notional_rate` | `87.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|APE/USD|sell_notional_rate` | `78.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|buy_notional_rate` | `90.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|gross_notional_rate_baseline` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|gross_notional_rate_divergence` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|gross_notional_rate_ratio` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|gross_notional_rate_velocity` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|midpoint_return_rate` | `90.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|midpoint_return_rate_baseline` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|midpoint_return_rate_divergence` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|net_notional_rate` | `90.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|net_notional_rate_velocity` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|sell_notional_rate` | `90.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ATH/USD|buy_notional_rate` | `76.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ATH/USD|gross_notional_rate_baseline` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ATH/USD|gross_notional_rate_divergence` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ATH/USD|gross_notional_rate_ratio` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 2804 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|ARKM/USD|covariance_score_velocity` | `90.9%` | `90.0%` | `0.0325` | `COLD_START` |
| `correlation|ATH/USD|covariance_score_velocity` | `76.9%` | `90.0%` | `-0.0426` | `COLD_START` |
| `correlation|CAKE/USD|relative_return_energy_velocity` | `66.7%` | `87.5%` | `-0.0298` | `COLD_START` |
| `correlation|CRO/USD|absolute_covariance_score` | `50.0%` | `80.0%` | `0.4165` | `COLD_START` |
| `correlation|CRO/USD|cohort_absolute_covariance_score` | `50.0%` | `80.0%` | `0.1591` | `COLD_START` |
| `correlation|CRO/USD|cohort_covariance_score` | `50.0%` | `80.0%` | `0.4395` | `COLD_START` |
| `correlation|CRO/USD|cohort_covariance_score_dispersion` | `50.0%` | `80.0%` | `0.0151` | `COLD_START` |
| `correlation|CRO/USD|cohort_effective_peer_count` | `50.0%` | `80.0%` | `0.4539` | `COLD_START` |
| `correlation|CRO/USD|cohort_peer_count` | `50.0%` | `80.0%` | `0.2985` | `COLD_START` |
| `correlation|CRO/USD|covariance` | `50.0%` | `80.0%` | `0.6198` | `COLD_START` |
| `correlation|CRO/USD|covariance_p_value` | `50.0%` | `80.0%` | `-0.3954` | `COLD_START` |
| `correlation|CRO/USD|covariance_score` | `50.0%` | `80.0%` | `0.4165` | `COLD_START` |
| `correlation|CRO/USD|covariance_score_baseline` | `50.0%` | `80.0%` | `0.6782` | `COLD_START` |
| `correlation|CRO/USD|covariance_score_divergence` | `50.0%` | `80.0%` | `0.1811` | `COLD_START` |
| `correlation|CRO/USD|covariance_score_zscore` | `50.0%` | `80.0%` | `0.0736` | `COLD_START` |
| ... and 2125 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `5585895`
- Direct relationships: `2835292`; inverse relationships: `2750603`
- Signed real mean: `0.016`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.443`
- Real `|r|` above that empirical bound: `14.0%`
- KS distance in `|r|` space: `0.127`

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

- Held-out emissions: `5501` across `12` regions
- Excitation strength: mean `17155.816`, peak `19759538.384`, runner-up margin `17103.376`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `20.9%`
- Real transition entropy: `2.686` bits
- Empirical dwell-block null mean: `3.106` bits
- Difference (null - real): `0.420` bits

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

- Replayed ticks: `12009`
- Tokens verified: `13636`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/var/folders/30/bdbpd0hj5wddgkxyx8pbc0mc0000gn/T/go-build436659713/b001/exe/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `22720`
- Physical invariant violations: `0`
- Same-time ambiguous trades skipped: `11558`
- Comparisons per check: `map[buy_notional_rate:11000 cvd_coverage:11162 midpoint_log_return:11038 rate_without_elapsed_time:3466 response_midpoint:at:11038 response_midpoint:from:11038 sell_notional_rate:11000 signed_net_fraction_bounded:10354 trade_fields:22720 trade_rate:11000]`
- Violations per check: `map[]`
- Unexercised checks: `[]`
- Verdict: `VALID`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `113458`
- Lookahead leakage detected: `false` (compared: `2588664`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.044` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.046` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.080` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.002` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.001` bits (dominant: `false`)

