# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791947694536206000` | **Symbol:** `ALL` | **Ticks:** `5861` | **Generated:** `2026-10-14T03:34:52Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **VALID** | declared hard domains on every observation; bounded SNM in [-1, 1]; valid state | 0/48 domain breaches; 0 norm breaches; state pass=true |
| **0.5 Timing** | **VALID** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 0 spikes of 57774 |
| **1. Vitality** | **MEASURED** | descriptive | 8270 raw series, 8270 canonical cells, 1104 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 78779 pairs, 13.0% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.022 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 2345 emissions, H=2.986, p=0.020 |
| **5a. Ignition precursors** | **NOT_SUPPORTED** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.143 bits, p=0.627 |
| **5b. Exhaustion precursors** | **INSUFFICIENT_DATA** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5c. Held-out precursor skill** | **INSUFFICIENT_DATA** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 328 held-out, MCC 0.000, p=0.000 |
| **5d. Friction clearance** | **NOT_A_TEST** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **NOT_SUPPORTED** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 33.3% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **CONTRACT_BREACH** | cvd frames recomputed from the stored trade tape; every check exercised | 109 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 121457 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.010 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `48`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `0` observations beyond rank bound |z| <= rankBound(support) (max |z|=`1.00`, saturated=`4.2%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `57774`
- Ingestion latency (Timestamp - At): mean `-39.6ms`, p95 `-3.8ms`, max `116.5ms`
- Latency spikes: `0`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `VALID`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `8270` (varying `6621`, constant `1074`, warming `564`)
- Canonical grid cells: `8270` (varying `6591`, constant `1104`, warming `564`)
- High-correlation canonical pairs shown by the current reference filter: `460`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|ESP/USD|cohort_covariance_score` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|QUID/USD|cohort_covariance_score` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|TON/USD|cohort_covariance_score` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|VSN/USD|cohort_covariance_score` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|AERO/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ALCX/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ALCX/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|APR/USD|signed_net_fraction` | `88.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ATLAS/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ATLAS/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLUE/USD|gross_notional` | `60.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLUE/USD|signed_net_fraction` | `60.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BONK/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BONK/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CTC/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CTC/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOVU/USD|gross_notional` | `20.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOVU/USD|signed_net_fraction` | `20.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ESP/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ETC/USD|flow_aligned_midpoint_return` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ETC/USD|midpoint_log_return` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|EV/USD|signed_net_fraction` | `98.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|HNT/USD|gross_notional` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|HNT/USD|signed_net_fraction` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|IDEX/USD|flow_aligned_midpoint_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|IDEX/USD|gross_notional` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|IDEX/USD|midpoint_log_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|IDEX/USD|signed_net_fraction` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KSM/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KSM/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KTA/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KULA/USD|flow_aligned_midpoint_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KULA/USD|midpoint_log_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LDO/USD|flow_aligned_midpoint_return` | `87.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LDO/USD|midpoint_log_return` | `87.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LIGHTER/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LSK/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LSK/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MANA/USD|signed_net_fraction` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MINA/USD|signed_net_fraction` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MLN/USD|signed_net_fraction` | `94.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MON/USD|gross_notional` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MON/USD|signed_net_fraction` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|NES/USD|signed_net_fraction` | `60.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|OMI/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|OMI/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PLUME/USD|gross_notional` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PLUME/USD|signed_net_fraction` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PNUT/USD|signed_net_fraction` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|QUID/USD|signed_net_fraction` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 1054 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|GRT/USD|cohort_covariance_score` | `77.8%` | `85.7%` | `0.0797` | `COLD_START` |
| `correlation|ZRO/USD|cohort_covariance_score` | `85.7%` | `83.3%` | `0.0707` | `COLD_START` |
| `cvd|ADA/USD|signed_net_fraction` | `77.8%` | `92.9%` | `0.0526` | `COLD_START` |
| `cvd|AKT/USD|signed_net_fraction` | `85.7%` | `91.7%` | `-0.0556` | `COLD_START` |
| `cvd|ALEO/USD|signed_net_fraction` | `92.3%` | `83.3%` | `0.0252` | `COLD_START` |
| `cvd|APT/USD|signed_net_fraction` | `91.2%` | `87.1%` | `0.0135` | `COLD_START` |
| `cvd|AVA/USD|signed_net_fraction` | `98.0%` | `87.0%` | `-0.0018` | `COLD_START` |
| `cvd|BAT/USD|signed_net_fraction` | `98.0%` | `80.6%` | `-0.0054` | `COLD_START` |
| `cvd|BCH/USD|signed_net_fraction` | `90.6%` | `82.8%` | `0.0068` | `COLD_START` |
| `cvd|BTC/USD|signed_net_fraction` | `99.4%` | `85.5%` | `0.0005` | `COLD_START` |
| `cvd|CAP/USD|signed_net_fraction` | `99.7%` | `81.4%` | `0.0038` | `COLD_START` |
| `cvd|CC/USD|signed_net_fraction` | `88.9%` | `85.2%` | `0.0041` | `COLD_START` |
| `cvd|DASH/USD|signed_net_fraction` | `97.2%` | `95.7%` | `-0.0097` | `COLD_START` |
| `cvd|DGAI/USD|signed_net_fraction` | `92.9%` | `84.6%` | `-0.0156` | `COLD_START` |
| `cvd|DOG/USD|signed_net_fraction` | `61.8%` | `91.5%` | `-0.0256` | `COLD_START` |
| ... and 317 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `78779`
- Direct relationships: `40021`; inverse relationships: `38732`
- Signed real mean: `0.013`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.419`
- Real `|r|` above that empirical bound: `13.0%`
- KS distance in `|r|` space: `0.133`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid stationarity & completeness

- Cross-half excitation JSD: `0.022` bits (TVD `0.128`, p=`0.020`)
- Frames observed by the grid: `5849` of `5861` (`99.8%`)

![Grid Completeness](plots/stage3_grid_completeness.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `2345` across `15` regions
- Excitation strength: mean `1.356`, peak `6.688`, runner-up margin `0.483`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `21.8%`
- Real transition entropy: `2.986` bits
- Empirical dwell-block null mean: `3.056` bits
- Difference (null - real): `0.070` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `8` (chop, down, flat, up, up_friction)
- A->B Ignition: `NOT_SUPPORTED`, event/control `304/37`, JSD `0.143` vs null95 `0.954`
- B->C Exhaustion: `INSUFFICIENT_DATA`, event/control `0/49`, JSD `0.000` vs null95 `0.000`
- Supplemental non-excursion background observations: `5305`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `0.0%` | MCC: `0.000`
- Precision / Recall: `0.0%` / `0.0%`
- Mutual Information (Predictive Gain): `0.000` bits
- Prior Base Rate: `0.0%` | Top Precursor Tokens: ``

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `2` | Round-Trip Taker Fee: `160.00` bps
- Friction Clearance Rate: `50.0%` (`1` profitable / `1` unprofitable)
- Gross Mean Return: `3.72%` | Net Mean Return after Fees: `2.08%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

#### 4. Excursion Dynamic Resonance: Regional Transitions

Captured `5` representative market excursion transition profiles. Consecutive identical winning region tokens are run-length deduplicated into distinct directional transitions:

- **Excursion #1 (CAP/USD UP)**: `287` transitions across ticks [744 .. 10186] | Net Return: `+6.65%`
  - Sequence: `R13(1) -> R03(1) -> R11(1) -> R03(1) -> R04(1) -> R03(2) -> R11(1) -> R03(1) -> R11(2) -> R10(2) -> R11(1) -> R03(1) -> R09(1) -> R10(1) -> R11(1) -> R10(1) -> R03(1) -> R10(1) -> R06(1) -> R08(1) -> R09(1) -> R06(1) -> R05(1) -> R11(1) -> R10(1) -> R09(1) -> R11(1) -> R09(1) -> R06(1) -> R11(1) -> R03(1) -> R10(1) -> R11(1) -> R09(1) -> R01(1) -> R08(1) -> R09(1) -> R04(1) -> R09(1) -> R13(2) -> R10(2) -> R09(1) -> R10(1) -> R03(1) -> R11(2) -> R03(1) -> R11(1) -> R03(1) -> R09(2) -> R11(1) -> R10(1) -> R11(3) -> R09(2) -> R11(1) -> R06(1) -> R04(1) -> R13(1) -> R09(1) -> R10(1) -> R01(1) -> R04(2) -> R09(1) -> R03(1) -> R07(1) -> R03(1) -> R04(1) -> R10(1) -> R09(1) -> R01(1) -> R03(1) -> R09(1) -> R04(1) -> R09(1) -> R10(3) -> R03(1) -> R05(1) -> R04(1) -> R13(1) -> R04(1) -> R08(1) -> R04(1) -> R10(1) -> R09(1) -> R10(1) -> R03(1) -> R13(1) -> R04(1) -> R06(1) -> R08(2) -> R09(1) -> R01(1) -> R09(1) -> R01(1) -> R08(1) -> R10(1) -> R09(1) -> R06(1) -> R10(1) -> R13(1) -> R09(1) -> R03(1) -> R10(1) -> R04(2) -> R10(3) -> R08(1) -> R10(1) -> R04(1) -> R10(1) -> R04(1) -> R06(1) -> R03(2) -> R08(1) -> R10(3) -> R04(1) -> R03(1) -> R14(1) -> R10(1) -> R09(1) -> R01(1) -> R03(1) -> R09(1) -> R04(1) -> R09(1) -> R08(1) -> R11(1) -> R08(1) -> R11(1) -> R09(1) -> R10(1) -> R13(2) -> R09(1) -> R11(1) -> R09(1) -> R11(1) -> R03(1) -> R09(1) -> R13(1) -> R11(1) -> R08(1) -> R05(1) -> R10(4) -> R09(1) -> R04(2) -> R10(1) -> R04(1) -> R09(1) -> R04(1) -> R10(1) -> R03(1) -> R10(2) -> R04(1) -> R09(1) -> R10(3) -> R09(1) -> R11(1) -> R13(1) -> R14(1) -> R09(1) -> R10(1) -> R04(1) -> R03(1) -> R10(1) -> R03(1) -> R09(2) -> R11(1) -> R03(1) -> R04(1) -> R10(1) -> R09(3) -> R05(1) -> R08(1) -> R13(1) -> R09(1) -> R13(1) -> R09(1) -> R03(1) -> R09(2) -> R13(1) -> R11(1) -> R09(3) -> R14(1) -> R10(1) -> R09(2) -> R11(1) -> R09(1) -> R10(1) -> R09(1) -> R04(1) -> R10(1) -> R11(2) -> R14(1) -> R09(1) -> R03(1) -> R09(1) -> R13(1) -> R09(1) -> R03(2) -> R01(1) -> R10(2) -> R04(1) -> R10(1) -> R09(2) -> R03(1) -> R11(1) -> R09(1) -> R11(1) -> R09(1) -> R10(1) -> R01(1) -> R08(1) -> R12(1) -> R08(1) -> R13(2) -> R04(4) -> R06(1) -> R09(1) -> R04(2) -> R10(2) -> R04(1) -> R08(1) -> R11(1) -> R02(1) -> R14(1) -> R09(2) -> R13(1) -> R04(1) -> R10(2) -> R14(1) -> R10(1) -> R03(1) -> R13(1) -> R09(1) -> R03(1) -> R09(1) -> R03(1) -> R09(5) -> R11(2) -> R09(1) -> R11(2) -> R10(1) -> R09(1) -> R02(1) -> R10(1) -> R03(1) -> R13(3) -> R09(1) -> R07(1) -> R09(1) -> R03(1) -> R01(1) -> R14(1) -> R10(1) -> R11(1) -> R09(1) -> R13(1) -> R01(1) -> R14(2) -> R09(1) -> R10(4) -> R13(1) -> R01(1) -> R09(2) -> R03(1) -> R11(2) -> R09(1) -> R03(1) -> R11(1) -> R10(1) -> R09(1) -> R08(1) -> R04(1) -> R03(1) -> R09(1) -> R01(1) -> R08(1) -> R03(2) -> R11(3) -> R02(1) -> R03(1) -> R09(2) -> R11(1) -> R09(2) -> R11(1) -> R14(1) -> R09(1) -> R11(1) -> R13(1)`
- **Excursion #2 (AAVE/USD UP_FRICTION)**: `6` transitions across ticks [0 .. 1500] | Net Return: `+0.79%`
  - Sequence: `R00(1) -> R11(1) -> R01(1) -> R11(1) -> R08(1) -> R03(1)`
- **Excursion #3 (ALEO/USD CHOP)**: `11` transitions across ticks [2431 .. 5017] | Net Return: `-1.98%`
  - Sequence: `R00(1) -> R11(1) -> R14(1) -> R09(1) -> R11(1) -> R10(1) -> R13(1) -> R08(2) -> R10(1) -> R11(2) -> R10(1)`
- **Excursion #4 (US/USD DOWN)**: `27` transitions across ticks [0 .. 5119] | Net Return: `-2.09%`
  - Sequence: `R00(1) -> R11(2) -> R04(1) -> R08(1) -> R06(1) -> R09(2) -> R10(2) -> R13(1) -> R03(1) -> R08(1) -> R01(1) -> R11(2) -> R01(1) -> R09(3) -> R01(1) -> R11(4) -> R09(1) -> R11(1) -> R06(1) -> R09(1) -> R11(1) -> R03(1) -> R09(3) -> R10(1) -> R03(1) -> R11(1) -> R13(1)`
- **Excursion #5 (CT/USD DOWN)**: `30` transitions across ticks [0 .. 3955] | Net Return: `-2.17%`
  - Sequence: `R00(1) -> R01(1) -> R11(1) -> R10(1) -> R11(1) -> R03(1) -> R07(1) -> R03(2) -> R11(1) -> R01(1) -> R10(1) -> R03(1) -> R11(1) -> R09(1) -> R03(1) -> R11(2) -> R10(1) -> R11(2) -> R03(1) -> R11(1) -> R06(1) -> R11(1) -> R10(1) -> R13(1) -> R10(1) -> R03(1) -> R14(1) -> R11(1) -> R01(1) -> R10(1)`

![Excursion Dynamic Transitions Animation](plots/excursion_transitions.gif)

![Excursion Dynamic Transitions Summary](plots/excursion_transitions_summary.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `8` forming `7` sequential phases (enter: `1`, exit: `1`, wait: `5`)
- Balanced accuracy: `33.3%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `5/7` `71.4%`)
- Matthews Correlation Coefficient (MCC): `0.000`
- Enter action precision / recall: `0.0%` / `0.0%`
- Label-shuffled empirical null balanced accuracy: mean `27.7%`, 95th percentile `33.3%` (empirical p-value: `0.490`)
- Separates from null: `false`
- Post-teach memory retention: `7/7` (`100.0%`)
- Trie topology: `5` nodes, max depth `2`, mean depth `1.2`, branching factor `1.33`
- Basin geometry: records `4`, span `2`, active enter basins `1`, active exit basins `1` (total: `2`)
- Decisiveness: abstention rate `100.0%`, mean confidence `0.000`, mean contrast `0.000`
- Unseen background false-alarm rate: `2.35%` spurious triggers on continuous tape

#### Multi-Trie Empirical Comparison (Top-1 vs Top-2 vs Top-3)

| Configuration | Schema | Phases | Balanced Acc | Baseline Acc | MCC | Enter Prec / Rec | Retention | Nodes | Mean Depth | False Alarm | Null p-val |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Top-1 Winner (Rnn)** | `K=1` | 7 | `33.3%` | `33.3%` | `0.000` | `0.0%` / `0.0%` | `100.0%` | 5 | `1.2` | `2.35%` | `0.490` |
| **Top-2 Pair (Rnn-Rmm)** | `K=2` | 7 | `33.3%` | `33.3%` | `0.000` | `0.0%` / `0.0%` | `100.0%` | 6 | `1.5` | `0.00%` | `0.765` |
| **Top-3 Triad (Rnn-Rmm-Rpp)** | `K=3` | 7 | `33.3%` | `33.3%` | `0.000` | `0.0%` / `0.0%` | `100.0%` | 5 | `1.2` | `0.00%` | `0.745` |

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `5861`
- Tokens verified: `5849`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/Users/theapemachine/Library/Caches/go-build/62/62dcb4398899e876a137d9093c905f410eb3b5c55a70f18dc56fdc6d11802325-d/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `5861`
- Physical invariant violations: `109`
- Same-time ambiguous trades skipped: `2641`
- Comparisons per check: `map[cvd_coverage:3220 flow_aligned_midpoint_return:2964 level3_replay:1550501 midpoint_log_return:2964 signed_net_fraction_bounded:10620 trade_book:5652 trade_fields:5861]`
- Violations per check: `map[level3_replay:2 trade_book:107]`
- Unexercised checks: `[]`
- Verdict: `CONTRACT_BREACH`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `29310`
- Lookahead leakage detected: `false` (compared: `121457`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.118` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.009` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.014` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.016` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.005` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.004` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.051` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.098` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.150` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.000` bits (dominant: `false`)

