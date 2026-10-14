# SYMM Pipeline Empirical Audit

**State:** HYPOTHESES NOT SUPPORTED | **Epoch:** `1791947694536206000` | **Symbol:** `ALL` | **Ticks:** `964` | **Generated:** `2026-10-14T06:15:41Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **VALID** | declared hard domains on every observation; bounded SNM in [-1, 1]; valid state | 0/48 domain breaches; 0 norm breaches; state pass=true |
| **0.5 Timing** | **VALID** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 0 spikes of 8836 |
| **1. Vitality** | **MEASURED** | descriptive | 3390 raw series, 3390 canonical cells, 574 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 12147 pairs, 12.3% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.068 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 386 emissions, H=2.649, p=0.020 |
| **5a. Ignition precursors** | **NOT_SUPPORTED** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.739 bits, p=0.118 |
| **5b. Exhaustion precursors** | **INSUFFICIENT_DATA** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.000 bits, p=0.000 |
| **5c. Held-out precursor skill** | **INSUFFICIENT_DATA** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 27 held-out, MCC 0.000, p=0.000 |
| **5d. Friction clearance** | **NOT_A_TEST** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **NOT_SUPPORTED** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 16.7% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **VALID** | cvd frames recomputed from the stored trade tape; every check exercised | 0 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 13585 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.011 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `48`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `0` observations beyond rank bound |z| <= rankBound(support) (max |z|=`1.00`, saturated=`4.3%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `8836`
- Ingestion latency (Timestamp - At): mean `-39.8ms`, p95 `-1.0ms`, max `36.0ms`
- Latency spikes: `0`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `VALID`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `3390` (varying `2362`, constant `570`, warming `450`)
- Canonical grid cells: `3390` (varying `2358`, constant `574`, warming `450`)
- High-correlation canonical pairs shown by the current reference filter: `98`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|ICNT/USD|cohort_covariance_score` | `33.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ORCA/USD|cohort_covariance_score` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|QUID/USD|cohort_covariance_score` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|SOSO/USD|cohort_covariance_score` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|APR/USD|signed_net_fraction` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|AVAX/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BNB/USD|signed_net_fraction` | `83.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CT/USD|signed_net_fraction` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|flow_aligned_midpoint_return` | `92.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|midpoint_log_return` | `92.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DASH/USD|signed_net_fraction` | `85.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DGAI/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|EV/USD|signed_net_fraction` | `71.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ICNT/USD|gross_notional` | `33.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ICNT/USD|signed_net_fraction` | `33.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|IDEX/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|IDEX/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KAS/USD|flow_aligned_midpoint_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KAS/USD|midpoint_log_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LTC/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MANA/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MANA/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PEPE/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PUMP/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PUMP/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PUMP/USD|signed_net_fraction` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PUPS/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PUPS/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PYTH/USD|signed_net_fraction` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|Q/USD|signed_net_fraction` | `60.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|QUID/USD|signed_net_fraction` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|SUI/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|SUI/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|TAO/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|TAO/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|US/USD|signed_net_fraction` | `85.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|USELESS/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|USELESS/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WAL/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|WAL/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZK/USD|flow_aligned_midpoint_return` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ZK/USD|midpoint_log_return` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|BLUE/USD|book_imbalance` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|BLUE/USD|touch_imbalance` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|BLUE/USD|touch_notional:ask` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|BLUE/USD|touch_notional:bid` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|DOVU/USD|mutation_activity_imbalance` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|DOVU/USD|touch_notional:ask` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|IDEX/USD|book_imbalance` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|IDEX/USD|net_displayed_flow:bid` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 524 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|INJ/USD|cohort_covariance_score` | `83.3%` | `80.0%` | `0.0228` | `COLD_START` |
| `correlation|ZEUS/USD|cohort_covariance_score` | `82.4%` | `94.6%` | `-0.0064` | `COLD_START` |
| `cvd|APT/USD|signed_net_fraction` | `81.2%` | `92.3%` | `0.0615` | `COLD_START` |
| `cvd|AVA/USD|signed_net_fraction` | `80.0%` | `87.5%` | `-0.0623` | `COLD_START` |
| `cvd|BNB/USD|gross_notional` | `83.3%` | `80.0%` | `-0.0161` | `COLD_START` |
| `cvd|BTC/USD|signed_net_fraction` | `97.2%` | `87.9%` | `-0.0016` | `COLD_START` |
| `cvd|DOG/USD|signed_net_fraction` | `46.3%` | `84.0%` | `-0.0482` | `COLD_START` |
| `cvd|EV/USD|gross_notional` | `71.4%` | `80.0%` | `-0.0126` | `COLD_START` |
| `cvd|STRK/USD|signed_net_fraction` | `66.7%` | `83.3%` | `-0.1111` | `COLD_START` |
| `cvd|UAI/USD|signed_net_fraction` | `85.7%` | `83.3%` | `0.0000` | `COLD_START` |
| `cvd|UNI/USD|signed_net_fraction` | `77.8%` | `85.7%` | `-0.0947` | `COLD_START` |
| `cvd|XRP/USD|signed_net_fraction` | `90.0%` | `88.9%` | `0.0894` | `COLD_START` |
| `cvd|ZBCN/USD|signed_net_fraction` | `75.0%` | `91.7%` | `-0.0556` | `COLD_START` |
| `cvd|ZEUS/USD|signed_net_fraction` | `89.7%` | `96.7%` | `0.0014` | `COLD_START` |
| `depthflow|US/USD|touch_notional:bid` | `52.4%` | `81.8%` | `0.0189` | `COLD_START` |
| ... and 96 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `12147`
- Direct relationships: `6114`; inverse relationships: `6031`
- Signed real mean: `0.013`; signed shuffled mean: `-0.000`
- 95th percentile of `|null r|`: `0.508`
- Real `|r|` above that empirical bound: `12.3%`
- KS distance in `|r|` space: `0.131`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid stationarity & completeness

- Cross-half excitation JSD: `0.068` bits (TVD `0.253`, p=`0.020`)
- Frames observed by the grid: `952` of `964` (`98.8%`)

![Grid Completeness](plots/stage3_grid_completeness.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `386` across `14` regions
- Excitation strength: mean `1.332`, peak `6.710`, runner-up margin `0.486`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `22.3%`
- Real transition entropy: `2.649` bits
- Empirical dwell-block null mean: `2.817` bits
- Difference (null - real): `0.168` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `20` (chop, down, flat, up, up_friction)
- A->B Ignition: `NOT_SUPPORTED`, event/control `1/27`, JSD `0.739` vs null95 `0.739`
- B->C Exhaustion: `INSUFFICIENT_DATA`, event/control `0/1`, JSD `0.000` vs null95 `0.000`
- Supplemental non-excursion background observations: `773`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `0.0%` | MCC: `0.000`
- Precision / Recall: `0.0%` / `0.0%`
- Mutual Information (Predictive Gain): `0.000` bits
- Prior Base Rate: `0.0%` | Top Precursor Tokens: ``

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `7` | Round-Trip Taker Fee: `160.00` bps
- Friction Clearance Rate: `14.3%` (`1` profitable / `6` unprofitable)
- Gross Mean Return: `1.51%` | Net Mean Return after Fees: `-0.10%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

#### 4. Excursion Dynamic Resonance: Regional Transitions

Captured `5` representative market excursion transition profiles. Consecutive identical winning region tokens are run-length deduplicated into distinct directional transitions:

- **Excursion #1 (XCN/USD CHOP)**: `42` transitions across ticks [0 .. 74366] | Net Return: `+0.68%`
  - Sequence: `R00(1) -> R14(2) -> R01(2) -> R09(1) -> R11(1) -> R03(2) -> R11(1) -> R03(4) -> R06(1) -> R11(1) -> R09(2) -> R11(2) -> R14(1) -> R11(1) -> R01(1) -> R09(1) -> R11(1) -> R09(1) -> R13(1) -> R09(1) -> R11(1) -> R14(1) -> R09(1) -> R11(1) -> R10(1) -> R09(1) -> R11(1) -> R10(1) -> R11(1) -> R10(1) -> R09(1) -> R10(1) -> R11(1) -> R10(1) -> R03(1) -> R11(1) -> R03(1) -> R10(1) -> R11(1) -> R08(1) -> R11(1) -> R09(1)`
- **Excursion #2 (STX/USD UP_FRICTION)**: `5` transitions across ticks [0 .. 15758] | Net Return: `+0.59%`
  - Sequence: `R00(1) -> R11(2) -> R09(1) -> R04(1) -> R14(1)`
- **Excursion #3 (DOT/USD UP_FRICTION)**: `10` transitions across ticks [0 .. 1375] | Net Return: `+0.20%`
  - Sequence: `R00(1) -> R14(1) -> R09(1) -> R04(1) -> R11(1) -> R01(1) -> R14(1) -> R10(1) -> R07(1) -> R11(1)`
- **Excursion #4 (MEGA/USD UP_FRICTION)**: `4` transitions across ticks [526 .. 1328] | Net Return: `+0.07%`
  - Sequence: `R10(1) -> R09(1) -> R03(2) -> R11(1)`
- **Excursion #5 (APT/USD UP_FRICTION)**: `5` transitions across ticks [351 .. 521] | Net Return: `+0.01%`
  - Sequence: `R14(1) -> R13(1) -> R03(1) -> R11(1) -> R09(1)`

![Excursion Dynamic Transitions Animation](plots/excursion_transitions.gif)

![Excursion Dynamic Transitions Summary](plots/excursion_transitions_summary.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `20` forming `8` sequential phases (enter: `1`, exit: `1`, wait: `6`)
- Balanced accuracy: `16.7%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `3/8` `37.5%`)
- Matthews Correlation Coefficient (MCC): `-0.293`
- Enter action precision / recall: `0.0%` / `0.0%`
- Label-shuffled empirical null balanced accuracy: mean `29.8%`, 95th percentile `33.3%` (empirical p-value: `1.000`)
- Separates from null: `false`
- Post-teach memory retention: `5/8` (`62.5%`)
- Trie topology: `4` nodes, max depth `2`, mean depth `1.0`, branching factor `1.50`
- Basin geometry: records `3`, span `2`, active enter basins `1`, active exit basins `1` (total: `2`)
- Decisiveness: abstention rate `62.5%`, mean confidence `0.375`, mean contrast `0.375`
- Unseen background false-alarm rate: `17.10%` spurious triggers on continuous tape

#### Multi-Trie Empirical Comparison (Top-1 vs Top-2 vs Top-3)

| Configuration | Schema | Phases | Balanced Acc | Baseline Acc | MCC | Enter Prec / Rec | Retention | Nodes | Mean Depth | False Alarm | Null p-val |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Top-1 Winner (Rnn)** | `K=1` | 8 | `16.7%` | `33.3%` | `-0.293` | `0.0%` / `0.0%` | `62.5%` | 4 | `1.0` | `17.10%` | `1.000` |
| **Top-2 Pair (Rnn-Rmm)** | `K=2` | 7 | `41.7%` | `50.0%` | `-0.167` | `0.0%` / `0.0%` | `85.7%` | 2 | `0.5` | `3.11%` | `1.000` |
| **Top-3 Triad (Rnn-Rmm-Rpp)** | `K=3` | 7 | `50.0%` | `50.0%` | `0.000` | `0.0%` / `0.0%` | `100.0%` | 2 | `0.5` | `3.11%` | `0.902` |

> ⚠️ **LEARNING DEFICIT:** Prequential balanced accuracy (16.7%) trails baseline policy (33.3%). Memory retention is at 62.5%.

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `964`
- Tokens verified: `952`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/Users/theapemachine/Library/Caches/go-build/e1/e17f32e8ee991bc340d73e65d2d39f8abf4d98a2dec9ca73a00ed9946ea3fea3-d/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `964`
- Physical invariant violations: `0`
- Same-time ambiguous trades skipped: `413`
- Comparisons per check: `map[cvd_coverage:551 flow_aligned_midpoint_return:382 level3_replay:147281 midpoint_log_return:382 signed_net_fraction_bounded:1402 trade_book:763 trade_fields:964]`
- Violations per check: `map[]`
- Unexercised checks: `[]`
- Verdict: `VALID`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `4820`
- Lookahead leakage detected: `false` (compared: `13585`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.085` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.008` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.025` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.016` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.011` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.048` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.080` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.194` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.000` bits (dominant: `false`)

