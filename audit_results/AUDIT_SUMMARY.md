# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791786130376447000` | **Symbol:** `ALL` | **Ticks:** `78257` | **Generated:** `2026-10-12T09:23:46Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **VALID** | declared hard domains on every observation; |z| <= rankBound(support); valid state & mature rank support | 0/363 domain breaches; 0 unstandardized mature; 0 z beyond bound; state pass=true |
| **0.5 Timing** | **VALID** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 3209 spikes of 878565 |
| **1. Vitality** | **MEASURED** | descriptive | 94883 raw series, 94883 canonical cells, 1126 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 10875367 pairs, 18.0% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.011 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 14054 emissions, H=2.462, p=0.020 |
| **5a. Ignition precursors** | **NOT_SUPPORTED** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.040 bits, p=0.961 |
| **5b. Exhaustion precursors** | **NOT_SUPPORTED** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.148 bits, p=0.882 |
| **5c. Held-out precursor skill** | **INSUFFICIENT_DATA** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 1742 held-out, MCC 0.000, p=0.000 |
| **5d. Friction clearance** | **NOT_A_TEST** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **NOT_SUPPORTED** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 37.0% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **CONTRACT_BREACH** | cvd frames recomputed from the stored trade tape; every check exercised | 3721 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 10829349 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.012 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `363`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `0` observations beyond rank bound |z| <= rankBound(support) (max |z|=`3.88`, saturated=`2.2%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `878565`
- Ingestion latency (Timestamp - At): mean `-57.1ms`, p95 `-8.1ms`, max `2144.1ms`
- Latency spikes: `3209`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `VALID`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `94883` (varying `89105`, constant `1083`, warming `4695`)
- Canonical grid cells: `94883` (varying `75020`, constant `1126`, warming `18737`)
- High-correlation canonical pairs shown by the current reference filter: `98237`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|HFT/USD|covariance_score_velocity` | `77.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|HFT/USD|covariance_standard_error` | `77.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|HFT/USD|overlap_density` | `77.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|LDO/USD|relative_return_energy` | `45.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|LDO/USD|return_energy_rate:measured` | `45.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|PNUT/USD|relative_return_energy` | `63.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|PNUT/USD|return_energy_rate:measured` | `63.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|SWELL/USD|relative_return_energy` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|SWELL/USD|return_energy_rate:measured` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|TMX/USD|overlap_density` | `79.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|TRIA/USD|cohort_absolute_covariance_score` | `70.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|TRIA/USD|cohort_covariance_score_dispersion` | `70.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|TRIA/USD|relative_return_energy_baseline` | `70.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|TRIA/USD|relative_return_energy_divergence` | `70.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ZCHF/USD|relative_return_energy` | `72.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ZCHF/USD|return_energy_rate:measured` | `72.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|aggressive_notional:sell` | `91.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|executed_quantity:sell` | `91.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|sell_notional_rate` | `91.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|signed_count_fraction` | `91.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|signed_net_fraction` | `91.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|signed_net_fraction_baseline` | `91.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|signed_net_fraction_divergence` | `91.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|trade_count:sell` | `91.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DAG/USD|gross_notional_rate_zscore` | `54.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WEMIX/USD|aggressive_notional:sell` | `57.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WEMIX/USD|executed_quantity:sell` | `57.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WEMIX/USD|sell_notional_rate` | `89.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WEMIX/USD|signed_count_fraction` | `57.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WEMIX/USD|signed_net_fraction` | `57.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WEMIX/USD|signed_net_fraction_baseline` | `55.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WEMIX/USD|signed_net_fraction_divergence` | `55.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WEMIX/USD|trade_count:sell` | `57.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZCHF/USD|aggressive_notional:buy` | `61.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZCHF/USD|buy_notional_rate` | `91.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZCHF/USD|executed_quantity:buy` | `61.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZCHF/USD|signed_count_fraction` | `61.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZCHF/USD|signed_net_fraction` | `61.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZCHF/USD|signed_net_fraction_baseline` | `61.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZCHF/USD|signed_net_fraction_divergence` | `61.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZCHF/USD|trade_count:buy` | `61.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AUSD/USD|background_rate` | `31.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AUSD/USD|background_rate:buy` | `31.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AUSD/USD|background_rate:sell` | `31.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AUSD/USD|branching_spectral_radius` | `31.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AUSD/USD|excitation_amplitude:buy_from_buy` | `31.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AUSD/USD|excitation_amplitude:buy_from_sell` | `31.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AUSD/USD|excitation_amplitude:sell_from_buy` | `31.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AUSD/USD|excitation_amplitude:sell_from_sell` | `31.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AUSD/USD|excitation_decay` | `31.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 1076 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|0G/USD|historical_path_percentile` | `63.2%` | `83.3%` | `0.1011` | `COLD_START` |
| `correlation|ACH/USD|last_price` | `93.8%` | `93.3%` | `0.0200` | `COLD_START` |
| `correlation|ACH/USD|observation_count` | `93.8%` | `93.3%` | `0.1260` | `COLD_START` |
| `correlation|APENFT/USD|absolute_covariance_score` | `70.8%` | `94.1%` | `-0.0318` | `COLD_START` |
| `correlation|APENFT/USD|cohort_absolute_covariance_score` | `70.8%` | `94.1%` | `-0.0269` | `COLD_START` |
| `correlation|APENFT/USD|cohort_covariance_score` | `70.8%` | `94.1%` | `0.1111` | `COLD_START` |
| `correlation|APENFT/USD|cohort_covariance_score_dispersion` | `70.8%` | `94.1%` | `-0.0269` | `COLD_START` |
| `correlation|APENFT/USD|cohort_effective_peer_count` | `70.8%` | `94.1%` | `0.1111` | `COLD_START` |
| `correlation|APENFT/USD|cohort_peer_count` | `70.8%` | `94.1%` | `0.1111` | `COLD_START` |
| `correlation|APENFT/USD|covariance` | `70.8%` | `94.1%` | `-0.0920` | `COLD_START` |
| `correlation|APENFT/USD|covariance_p_value` | `70.8%` | `94.1%` | `0.0318` | `COLD_START` |
| `correlation|APENFT/USD|covariance_score` | `70.8%` | `94.1%` | `-0.0424` | `COLD_START` |
| `correlation|APENFT/USD|covariance_score_baseline` | `70.8%` | `94.1%` | `0.1111` | `COLD_START` |
| `correlation|APENFT/USD|covariance_score_divergence` | `70.8%` | `94.1%` | `0.0795` | `COLD_START` |
| `correlation|APENFT/USD|covariance_score_velocity` | `70.8%` | `94.1%` | `0.0370` | `COLD_START` |
| ... and 4318 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `10875367`
- Direct relationships: `5620089`; inverse relationships: `5255278`
- Signed real mean: `0.017`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.365`
- Real `|r|` above that empirical bound: `18.0%`
- KS distance in `|r|` space: `0.197`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid reproducibility & completeness

- Early grid: `363` cells / `12` regions
- Late grid: `362` cells / `12` regions
- Shared universe: `362` cells (`99.7%`)
- Adjusted Rand Index: `1.000`

> Balanced region sizes are enforced by the partitioner and are not presented as empirical evidence. No ARI health cutoff is applied.

![Stage 3 Partitioning](plots/stage3_region_partitioning.png)

#### Grid Completeness & Regional Availability

- Total symbol-tick frames evaluated: `86385`
- **Complete frames (all 12 regions active)**: `39682` (`45.9%`)
- **Incomplete frames (1..11 regions active)**: `42117` (`48.8%`)
- **Empty frames (0 regions active)**: `4586` (`5.3%`)
- Major pair completeness: `40.6%` | Altcoin completeness: `47.1%`

##### Active Region Cardinality Distribution

| Active Regions (K) | Frame Count | Percentage | Cumulative >= K |
| :---: | ---: | ---: | ---: |
| **K = 12** | 39682 | `45.9%` | `45.9%` |
| **K = 11** | 5691 | `6.6%` | `52.5%` |
| **K = 10** | 2488 | `2.9%` | `55.4%` |
| **K = 9** | 11544 | `13.4%` | `68.8%` |
| **K = 8** | 5874 | `6.8%` | `75.6%` |
| **K = 7** | 2148 | `2.5%` | `78.1%` |
| **K = 6** | 3869 | `4.5%` | `82.5%` |
| **K = 5** | 492 | `0.6%` | `83.1%` |
| **K = 4** | 629 | `0.7%` | `83.8%` |
| **K = 3** | 7997 | `9.3%` | `93.1%` |
| **K = 2** | 644 | `0.7%` | `93.8%` |
| **K = 1** | 741 | `0.9%` | `94.7%` |
| **K = 0** | 4586 | `5.3%` | `0.0%` |

> **Top Dropout Bottlenecks:** R11 (45.3% absent), R09 (42.1% absent), R05 (36.2% absent), R10 (33.4% absent)

![Grid Completeness](plots/stage3_grid_completeness.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `14054` across `12` regions
- Excitation strength: mean `4.059`, peak `54.162`, runner-up margin `1.446`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `30.4%`
- Real transition entropy: `2.462` bits
- Empirical dwell-block null mean: `3.079` bits
- Difference (null - real): `0.617` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `1397` (chop, down, flat, up, up_friction)
- A->B Ignition: `NOT_SUPPORTED`, event/control `127/2568`, JSD `0.040` vs null95 `0.306`
- B->C Exhaustion: `NOT_SUPPORTED`, event/control `89/190`, JSD `0.148` vs null95 `0.218`
- Supplemental non-excursion background observations: `30636`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `0.0%` | MCC: `0.000`
- Precision / Recall: `0.0%` / `0.0%`
- Mutual Information (Predictive Gain): `0.000` bits
- Prior Base Rate: `0.0%` | Top Precursor Tokens: ``

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `298` | Round-Trip Taker Fee: `160.00` bps
- Friction Clearance Rate: `10.7%` (`32` profitable / `266` unprofitable)
- Gross Mean Return: `0.72%` | Net Mean Return after Fees: `-0.88%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

#### 4. Excursion Dynamic Resonance: Regional Transitions

Captured `5` representative market excursion transition profiles. Consecutive identical winning region tokens are run-length deduplicated into distinct directional transitions:

- **Excursion #1 (ZEUS/USD UP)**: `5` transitions across ticks [0 .. 7318] | Net Return: `+8.25%`
  - Sequence: `R08(1) -> R06(2) -> R03(1) -> R06(1) -> R08(2)`
- **Excursion #2 (W/USD UP)**: `28` transitions across ticks [0 .. 12709] | Net Return: `+6.72%`
  - Sequence: `R10(1) -> R01(1) -> R10(12) -> R00(1) -> R01(1) -> R10(3) -> R01(1) -> R04(1) -> R10(2) -> R07(2) -> R10(25) -> R01(1) -> R08(1) -> R01(2) -> R06(1) -> R00(3) -> R10(1) -> R00(4) -> R04(1) -> R00(1) -> R06(1) -> R04(2) -> R00(1) -> R06(1) -> R07(1) -> R06(1) -> R07(3) -> R00(8)`
- **Excursion #3 (US/USD UP)**: `106` transitions across ticks [435 .. 17160] | Net Return: `+4.67%`
  - Sequence: `R01(1) -> R07(2) -> R00(1) -> R01(1) -> R04(1) -> R06(2) -> R04(1) -> R07(2) -> R10(1) -> R04(1) -> R07(1) -> R03(1) -> R10(2) -> R00(3) -> R06(1) -> R10(1) -> R11(1) -> R00(1) -> R11(5) -> R00(1) -> R11(2) -> R09(1) -> R03(1) -> R00(1) -> R10(1) -> R08(1) -> R10(1) -> R02(1) -> R10(3) -> R02(1) -> R00(6) -> R11(1) -> R10(3) -> R00(2) -> R10(2) -> R09(1) -> R00(1) -> R09(1) -> R10(1) -> R09(1) -> R03(1) -> R10(2) -> R02(1) -> R10(18) -> R11(1) -> R09(2) -> R10(1) -> R09(1) -> R10(1) -> R09(1) -> R10(2) -> R09(1) -> R10(8) -> R11(1) -> R10(6) -> R11(1) -> R10(2) -> R11(2) -> R10(1) -> R02(1) -> R00(12) -> R02(1) -> R00(5) -> R02(1) -> R00(3) -> R02(1) -> R00(3) -> R05(1) -> R00(5) -> R10(1) -> R02(1) -> R05(1) -> R01(1) -> R08(2) -> R00(3) -> R07(1) -> R00(1) -> R02(1) -> R00(1) -> R08(1) -> R10(5) -> R08(1) -> R10(1) -> R08(1) -> R05(1) -> R10(18) -> R07(1) -> R10(3) -> R07(1) -> R10(1) -> R08(1) -> R07(1) -> R09(1) -> R11(1) -> R01(2) -> R10(1) -> R09(2) -> R10(1) -> R01(2) -> R08(1) -> R10(3) -> R07(1) -> R01(1) -> R11(1) -> R08(1) -> R09(1)`
- **Excursion #4 (S/USD UP)**: `17` transitions across ticks [0 .. 13252] | Net Return: `+3.78%`
  - Sequence: `R10(4) -> R07(1) -> R02(2) -> R00(3) -> R09(1) -> R05(1) -> R00(3) -> R09(1) -> R10(5) -> R01(1) -> R00(3) -> R10(1) -> R00(1) -> R10(2) -> R01(1) -> R10(1) -> R06(2)`
- **Excursion #5 (RSR/USD UP)**: `46` transitions across ticks [7300 .. 16386] | Net Return: `+3.63%`
  - Sequence: `R10(3) -> R02(1) -> R00(3) -> R10(10) -> R00(1) -> R07(1) -> R00(1) -> R04(1) -> R00(1) -> R10(2) -> R00(1) -> R10(1) -> R04(1) -> R10(5) -> R00(3) -> R08(1) -> R10(2) -> R04(1) -> R00(2) -> R06(2) -> R04(1) -> R10(3) -> R03(1) -> R10(1) -> R01(1) -> R10(2) -> R07(2) -> R01(1) -> R10(5) -> R07(1) -> R00(2) -> R10(1) -> R00(3) -> R05(1) -> R00(4) -> R06(1) -> R00(7) -> R02(11) -> R07(1) -> R02(1) -> R07(1) -> R02(10) -> R07(3) -> R02(1) -> R07(3) -> R02(1)`

![Excursion Dynamic Transitions Animation](plots/excursion_transitions.gif)

![Excursion Dynamic Transitions Summary](plots/excursion_transitions_summary.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `1397` forming `517` sequential phases (enter: `5`, exit: `7`, wait: `505`)
- Balanced accuracy: `37.0%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `489/517` `94.6%`)
- Matthews Correlation Coefficient (MCC): `-0.004`
- Enter action precision / recall: `0.0%` / `0.0%`
- Label-shuffled empirical null balanced accuracy: mean `31.4%`, 95th percentile `39.3%` (empirical p-value: `0.216`)
- Separates from null: `false`
- Post-teach memory retention: `480/517` (`92.8%`)
- Trie topology: `777` nodes, max depth `86`, mean depth `15.4`, branching factor `1.19`
- Basin geometry: records `776`, span `86`, active enter basins `5`, active exit basins `7` (total: `12`)
- Decisiveness: abstention rate `96.5%`, mean confidence `0.008`, mean contrast `0.253`
- Unseen background false-alarm rate: `1.07%` spurious triggers on continuous tape

#### Multi-Trie Empirical Comparison (Top-1 vs Top-2 vs Top-3)

| Configuration | Schema | Phases | Balanced Acc | Baseline Acc | MCC | Enter Prec / Rec | Retention | Nodes | Mean Depth | False Alarm | Null p-val |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Top-1 Winner (Rnn)** | `K=1` | 517 | `37.0%` | `33.3%` | `-0.004` | `0.0%` / `0.0%` | `92.8%` | 777 | `15.4` | `1.07%` | `0.216` |
| **Top-2 Pair (Rnn-Rmm)** | `K=2` | 517 | `33.1%` | `33.3%` | `-0.004` | `0.0%` / `0.0%` | `98.6%` | 1319 | `18.3` | `0.49%` | `0.216` |
| **Top-3 Triad (Rnn-Rmm-Rpp)** | `K=3` | 517 | `33.1%` | `33.3%` | `-0.004` | `0.0%` / `0.0%` | `99.0%` | 1647 | `18.8` | `0.19%` | `0.275` |

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `78257`
- Tokens verified: `33484`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/Users/theapemachine/Library/Caches/go-build/29/299787678eda7a031b8562617d7ae96d79af9223362081863dbb867a1a8513ff-d/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `91322`
- Physical invariant violations: `3721`
- Same-time ambiguous trades skipped: `46304`
- Comparisons per check: `map[buy_notional_rate:44660 cvd_coverage:45018 level3_replay:23319105 midpoint_log_return:44799 rate_without_elapsed_time:14393 response_midpoint:at:44799 response_midpoint:from:35656 sell_notional_rate:44660 signed_net_fraction_bounded:43797 trade_book:91305 trade_fields:91322 trade_rate:44660]`
- Violations per check: `map[level3_replay:7 trade_book:3714]`
- Unexercised checks: `[]`
- Verdict: `CONTRACT_BREACH`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `428385`
- Lookahead leakage detected: `false` (compared: `10829349`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.167` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.033` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.002` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.007` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.004` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.009` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.001` bits (dominant: `false`)

