# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791634624198637000` | **Symbol:** `ALL` | **Ticks:** `30107` | **Generated:** `2026-10-10T13:57:48Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **CONTRACT_BREACH** | declared hard domains on every observation; |z| <= sqrt(n/0.05) per stream | 0/352 series breach a domain (0 observations); 4748 z beyond bound |
| **0.5 Timing** | **CONTRACT_BREACH** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 91682 spikes of 415965 |
| **1. Vitality** | **MEASURED** | descriptive | 85795 raw series, 85795 canonical cells, 2989 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 8885278 pairs, 14.4% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.006 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 14258 emissions, H=2.725, p=0.020 |
| **5a. Ignition precursors** | **NOT_SUPPORTED** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.004 bits, p=0.961 |
| **5b. Exhaustion precursors** | **SUPPORTED** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.030 bits, p=0.020 |
| **5c. Held-out precursor skill** | **NOT_SUPPORTED** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 9494 held-out, MCC -0.058, p=1.000 |
| **5d. Friction clearance** | **NOT_A_TEST** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **NOT_SUPPORTED** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 32.0% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **VALID** | cvd frames recomputed from the stored trade tape; every check exercised | 0 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 4693147 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.002 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `352`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `4748` observations beyond Chebyshev bound |z| <= sqrt(N/0.05) (max |z|=`56873714.94`, saturated=`2.9%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `415965`
- Ingestion latency (Timestamp - At): mean `-137.2ms`, p95 `-5.1ms`, max `568.3ms`
- Latency spikes: `91682`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `CONTRACT_BREACH`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `85795` (varying `79522`, constant `1134`)
- Canonical grid cells: `85795` (varying `68107`, constant `2989`)
- High-correlation canonical pairs shown by the current reference filter: `90756`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|ALICE/USD|relative_return_energy` | `75.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ATH/USD|relative_return_energy` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ATH/USD|return_energy_rate:measured` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BLUR/USD|relative_return_energy_velocity` | `62.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BLUR/USD|return_energy_rate:reference` | `62.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BRL1/USD|covariance_score_velocity` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BRL1/USD|relative_return_energy_velocity` | `69.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|BTT/USD|relative_return_energy` | `73.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|CRO/USD|last_price` | `90.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|CRO/USD|observation_count` | `90.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|DCR/USD|relative_return_energy` | `78.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EDGEX/USD|last_price` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|EDGEX/USD|observation_count` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ELIZAOS/USD|relative_return_energy` | `69.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ELIZAOS/USD|return_energy_rate:measured` | `69.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|FLR/USD|relative_return_energy` | `43.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|FLR/USD|return_energy_rate:measured` | `43.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|KAITO/USD|relative_return_energy` | `91.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|KSM/USD|relative_return_energy` | `78.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|LAPTOP/USD|relative_return_energy` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|LDO/USD|relative_return_energy` | `71.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|LDO/USD|return_energy_rate:measured` | `71.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|LUNA/USD|relative_return_energy` | `78.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|absolute_covariance_score` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|cohort_absolute_covariance_score` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|cohort_covariance_score` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|cohort_effective_peer_count` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|cohort_peer_count` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|covariance` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|covariance_p_value` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|covariance_score` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|covariance_score_baseline` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|covariance_score_divergence` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|covariance_score_velocity` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|covariance_score_zscore` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|covariance_standard_error` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|historical_path_distance` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|historical_path_percentile` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|overlap_density` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|overlap_pair_count` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|peer_return_energy_rate` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|relative_return_energy` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|relative_return_energy_baseline` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|relative_return_energy_divergence` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|relative_return_energy_velocity` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|relative_return_energy_zscore` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|return_energy:measured` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|return_energy:reference` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|return_energy_rate:measured` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|MELANIA/USD|return_energy_rate:reference` | `81.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 2939 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|AIN/USD|absolute_covariance_score` | `83.3%` | `90.0%` | `-0.1037` | `COLD_START` |
| `correlation|AIN/USD|cohort_absolute_covariance_score` | `83.3%` | `90.0%` | `-0.1492` | `COLD_START` |
| `correlation|AIN/USD|cohort_covariance_score` | `83.3%` | `90.0%` | `-0.0003` | `COLD_START` |
| `correlation|AIN/USD|cohort_covariance_score_dispersion` | `83.3%` | `90.0%` | `-0.0842` | `COLD_START` |
| `correlation|AIN/USD|cohort_effective_peer_count` | `83.3%` | `90.0%` | `0.1282` | `COLD_START` |
| `correlation|AIN/USD|cohort_peer_count` | `83.3%` | `90.0%` | `0.1538` | `COLD_START` |
| `correlation|AIN/USD|covariance` | `83.3%` | `90.0%` | `0.0084` | `COLD_START` |
| `correlation|AIN/USD|covariance_p_value` | `83.3%` | `90.0%` | `0.1126` | `COLD_START` |
| `correlation|AIN/USD|covariance_score` | `83.3%` | `90.0%` | `0.0128` | `COLD_START` |
| `correlation|AIN/USD|covariance_score_baseline` | `83.3%` | `90.0%` | `0.0261` | `COLD_START` |
| `correlation|AIN/USD|covariance_score_divergence` | `83.3%` | `90.0%` | `-0.0072` | `COLD_START` |
| `correlation|AIN/USD|covariance_score_velocity` | `83.3%` | `90.0%` | `-0.0219` | `COLD_START` |
| `correlation|AIN/USD|covariance_score_zscore` | `83.3%` | `90.0%` | `-0.0662` | `COLD_START` |
| `correlation|AIN/USD|covariance_standard_error` | `83.3%` | `90.0%` | `-0.0613` | `COLD_START` |
| `correlation|AIN/USD|historical_path_distance` | `83.3%` | `90.0%` | `-0.0689` | `COLD_START` |
| ... and 3365 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `8885278`
- Direct relationships: `4525836`; inverse relationships: `4359442`
- Signed real mean: `0.017`; signed shuffled mean: `-0.000`
- 95th percentile of `|null r|`: `0.423`
- Real `|r|` above that empirical bound: `14.4%`
- KS distance in `|r|` space: `0.137`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid reproducibility

- Early grid: `352` cells / `12` regions
- Late grid: `352` cells / `12` regions
- Shared universe: `352` cells (`100.0%`)
- Adjusted Rand Index: `1.000`

> Balanced region sizes are enforced by the partitioner and are not presented as empirical evidence. No ARI health cutoff is applied.

![Stage 3](plots/stage3_region_partitioning.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `14258` across `12` regions
- Excitation strength: mean `4377.988`, peak `12038567.559`, runner-up margin `4345.464`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `34.4%`
- Real transition entropy: `2.725` bits
- Empirical dwell-block null mean: `2.964` bits
- Difference (null - real): `0.239` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `3555` (chop, down, flat, up, up_friction)
- A->B Ignition: `NOT_SUPPORTED`, event/control `1576/24342`, JSD `0.004` vs null95 `0.020`
- B->C Exhaustion: `SUPPORTED`, event/control `1179/1020`, JSD `0.030` vs null95 `0.020`
- Supplemental non-excursion background observations: `10549`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `44.9%` | MCC: `-0.058`
- Precision / Recall: `6.8%` / `35.0%`
- Mutual Information (Predictive Gain): `0.000` bits
- Prior Base Rate: `8.6%` | Top Precursor Tokens: `R00, R01, R06, R11`

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `703` | Round-Trip Taker Fee: `160.00` bps
- Friction Clearance Rate: `15.9%` (`112` profitable / `591` unprofitable)
- Gross Mean Return: `1.02%` | Net Mean Return after Fees: `-0.58%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `3555` forming `3358` sequential phases (enter: `90`, exit: `108`, wait: `3160`)
- Balanced accuracy: `32.0%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `2398/3358` `71.4%`)
- Matthews Correlation Coefficient (MCC): `0.034`
- Enter action precision / recall: `9.5%` / `2.2%`
- Label-shuffled empirical null balanced accuracy: mean `33.6%`, 95th percentile `36.3%` (empirical p-value: `0.824`)
- Separates from null: `false`
- Post-teach memory retention: `2116/3358` (`63.0%`)
- Trie topology: `11134` nodes, max depth `296`, mean depth `31.8`, branching factor `1.09`
- Basin geometry: records `11133`, span `296`, active enter basins `46`, active exit basins `88` (total: `134`)
- Decisiveness: abstention rate `75.2%`, mean confidence `0.021`, mean contrast `42.612`
- Unseen background false-alarm rate: `10.35%` spurious triggers on continuous tape

> ⚠️ **LEARNING DEFICIT:** Prequential balanced accuracy (32.0%) trails baseline policy (33.3%). Memory retention is at 63.0%.

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `30107`
- Tokens verified: `34557`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/Users/theapemachine/Library/Caches/go-build/d5/d59140adca76adeb9a0af01506f56d8de93adb67aac3e66bca53c47810b637da-d/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `41746`
- Physical invariant violations: `0`
- Same-time ambiguous trades skipped: `19140`
- Comparisons per check: `map[buy_notional_rate:22339 cvd_coverage:22606 midpoint_log_return:22417 rate_without_elapsed_time:5883 response_midpoint:at:22417 response_midpoint:from:22417 sell_notional_rate:22339 signed_net_fraction_bounded:19820 trade_fields:41746 trade_rate:22339]`
- Violations per check: `map[]`
- Unexercised checks: `[]`
- Verdict: `VALID`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `209922`
- Lookahead leakage detected: `false` (compared: `4693147`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.033` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.043` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.051` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.001` bits (dominant: `false`)

