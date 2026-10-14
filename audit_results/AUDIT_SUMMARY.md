# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791937244705562000` | **Symbol:** `ALL` | **Ticks:** `21301` | **Generated:** `2026-10-14T00:58:10Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **VALID** | declared hard domains on every observation; bounded SNM in [-1, 1]; valid state | 0/43 domain breaches; 0 norm breaches; state pass=true |
| **0.5 Timing** | **VALID** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 32 spikes of 212922 |
| **1. Vitality** | **MEASURED** | descriptive | 10692 raw series, 10692 canonical cells, 1069 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 95403 pairs, 11.6% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.014 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 8521 emissions, H=3.007, p=0.020 |
| **5a. Ignition precursors** | **INVALID_EXPERIMENT** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.264 bits, p=0.549 |
| **5b. Exhaustion precursors** | **INVALID_EXPERIMENT** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.337 bits, p=0.510 |
| **5c. Held-out precursor skill** | **INVALID_EXPERIMENT** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 27 held-out, MCC 0.000, p=1.000 |
| **5d. Friction clearance** | **NOT_A_TEST** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **NOT_SUPPORTED** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 33.3% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **CONTRACT_BREACH** | cvd frames recomputed from the stored trade tape; every check exercised | 3 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 367349 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.011 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `43`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `0` observations beyond rank bound |z| <= rankBound(support) (max |z|=`1.00`, saturated=`4.0%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `212922`
- Ingestion latency (Timestamp - At): mean `-89.5ms`, p95 `-52.0ms`, max `745.2ms`
- Latency spikes: `32`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `VALID`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `10692` (varying `8494`, constant `1001`, warming `829`)
- Canonical grid cells: `10692` (varying `8426`, constant `1069`, warming `829`)
- High-correlation canonical pairs shown by the current reference filter: `571`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|BLZ/USD|cohort_covariance_score` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|GFI/USD|cohort_covariance_score` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|REKT/USD|cohort_covariance_score` | `33.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|SPACE/USD|cohort_covariance_score` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ZRC/USD|cohort_covariance_score` | `12.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ZRC/USD|historical_path_distance` | `12.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|ZRC/USD|historical_path_percentile` | `12.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|2Z/USD|signed_net_fraction` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ALTHEA/USD|signed_net_fraction` | `82.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ANON/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ANON/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARC/USD|signed_net_fraction` | `77.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BILLY/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BILLY/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLUE/USD|flow_aligned_midpoint_return` | `93.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLUE/USD|midpoint_log_return` | `93.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLZ/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLZ/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|C98/USD|signed_net_fraction` | `60.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CFX/USD|signed_net_fraction` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CHEX/USD|gross_notional` | `83.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CHEX/USD|signed_net_fraction` | `83.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CSPR/USD|gross_notional` | `60.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CSPR/USD|signed_net_fraction` | `60.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CTC/USD|flow_aligned_midpoint_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CTC/USD|midpoint_log_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DCR/USD|signed_net_fraction` | `91.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DEEP/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DEEP/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DENT/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DENT/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOLO/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DOLO/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DYDX/USD|gross_notional` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DYDX/USD|signed_net_fraction` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|EDGE/USD|signed_net_fraction` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ETC/USD|gross_notional` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ETC/USD|signed_net_fraction` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|FUN/USD|signed_net_fraction` | `78.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|G/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|G/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|GFI/USD|gross_notional` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|GFI/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|GMT/USD|signed_net_fraction` | `77.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|JASMY/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LCX/USD|gross_notional` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LCX/USD|signed_net_fraction` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MET/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MLN/USD|signed_net_fraction` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|MOODENG/USD|signed_net_fraction` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 1019 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|GMT/USD|cohort_covariance_score` | `77.8%` | `85.7%` | `0.0148` | `COLD_START` |
| `correlation|WBTC/USD|cohort_covariance_score` | `90.9%` | `80.0%` | `0.0708` | `COLD_START` |
| `cvd|ADA/USD|signed_net_fraction` | `99.2%` | `87.0%` | `0.0104` | `COLD_START` |
| `cvd|AERO/USD|signed_net_fraction` | `97.2%` | `80.4%` | `0.0065` | `COLD_START` |
| `cvd|AI/USD|signed_net_fraction` | `85.0%` | `82.4%` | `0.0447` | `COLD_START` |
| `cvd|ALEO/USD|gross_notional` | `75.0%` | `88.9%` | `-0.0227` | `COLD_START` |
| `cvd|ALEO/USD|signed_net_fraction` | `75.0%` | `88.9%` | `0.0680` | `COLD_START` |
| `cvd|ALGO/USD|signed_net_fraction` | `95.6%` | `83.1%` | `0.0171` | `COLD_START` |
| `cvd|APR/USD|signed_net_fraction` | `92.0%` | `87.0%` | `0.0516` | `COLD_START` |
| `cvd|ATH/USD|signed_net_fraction` | `66.7%` | `85.7%` | `0.0043` | `COLD_START` |
| `cvd|AVA/USD|signed_net_fraction` | `87.0%` | `90.0%` | `0.0086` | `COLD_START` |
| `cvd|AVAX/USD|signed_net_fraction` | `99.0%` | `80.3%` | `-0.0145` | `COLD_START` |
| `cvd|AZTEC/USD|signed_net_fraction` | `96.6%` | `86.0%` | `-0.0087` | `COLD_START` |
| `cvd|BCH/USD|signed_net_fraction` | `74.5%` | `80.5%` | `0.0265` | `COLD_START` |
| `cvd|BERA/USD|signed_net_fraction` | `89.5%` | `94.1%` | `0.0392` | `COLD_START` |
| ... and 582 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `95403`
- Direct relationships: `48166`; inverse relationships: `47126`
- Signed real mean: `0.008`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.412`
- Real `|r|` above that empirical bound: `11.6%`
- KS distance in `|r|` space: `0.121`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid stationarity & completeness

- Cross-half excitation JSD: `0.014` bits (TVD `0.082`, p=`0.020`)
- Frames observed by the grid: `21283` of `21301` (`99.9%`)

![Grid Completeness](plots/stage3_grid_completeness.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `8521` across `12` regions
- Excitation strength: mean `1.109`, peak `4.547`, runner-up margin `0.337`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `19.8%`
- Real transition entropy: `3.007` bits
- Empirical dwell-block null mean: `3.050` bits
- Difference (null - real): `0.043` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `10` (chop, down, flat, up, up_friction)
- A->B Ignition: `INVALID_EXPERIMENT`, event/control `15/34`, JSD `0.264` vs null95 `0.599`
- B->C Exhaustion: `INVALID_EXPERIMENT`, event/control `32/22`, JSD `0.337` vs null95 `0.337`
- Supplemental non-excursion background observations: `21071`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `50.0%` | MCC: `0.000`
- Precision / Recall: `0.0%` / `0.0%`
- Mutual Information (Predictive Gain): `0.000` bits
- Prior Base Rate: `55.6%` | Top Precursor Tokens: ``

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `4` | Round-Trip Taker Fee: `160.00` bps
- Friction Clearance Rate: `25.0%` (`1` profitable / `3` unprofitable)
- Gross Mean Return: `0.94%` | Net Mean Return after Fees: `-0.66%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

#### 4. Excursion Dynamic Resonance: Regional Transitions

Captured `5` representative market excursion transition profiles. Consecutive identical winning region tokens are run-length deduplicated into distinct directional transitions:

- **Excursion #1 (CAP/USD UP)**: `51` transitions across ticks [0 .. 8629] | Net Return: `+2.02%`
  - Sequence: `R00(1) -> R06(1) -> R03(1) -> R02(1) -> R05(1) -> R04(1) -> R11(1) -> R01(1) -> R08(1) -> R11(1) -> R01(1) -> R10(1) -> R08(1) -> R11(1) -> R06(1) -> R01(1) -> R03(1) -> R11(2) -> R08(1) -> R03(1) -> R05(1) -> R01(1) -> R06(1) -> R11(1) -> R05(1) -> R03(1) -> R01(2) -> R03(1) -> R06(1) -> R11(1) -> R01(2) -> R08(1) -> R03(1) -> R07(1) -> R01(1) -> R03(1) -> R01(1) -> R03(3) -> R10(1) -> R04(1) -> R01(1) -> R08(1) -> R07(1) -> R03(1) -> R10(1) -> R04(1) -> R09(1) -> R11(1) -> R03(3) -> R05(1) -> R03(2)`
- **Excursion #2 (DMC/USD UP)**: `9` transitions across ticks [0 .. 3327] | Net Return: `+0.40%`
  - Sequence: `R00(2) -> R03(1) -> R10(1) -> R03(1) -> R01(1) -> R08(1) -> R03(1) -> R08(1) -> R03(1)`
- **Excursion #3 (PYTH/USD CHOP)**: `33` transitions across ticks [0 .. 8354] | Net Return: `+0.91%`
  - Sequence: `R00(1) -> R08(1) -> R07(1) -> R03(2) -> R10(1) -> R09(1) -> R10(1) -> R11(1) -> R01(1) -> R03(2) -> R10(1) -> R04(1) -> R01(1) -> R10(1) -> R05(1) -> R10(3) -> R05(1) -> R01(3) -> R05(1) -> R01(1) -> R03(1) -> R05(1) -> R10(1) -> R03(1) -> R05(1) -> R07(1) -> R03(2) -> R10(1) -> R03(1) -> R10(1) -> R07(2) -> R10(1) -> R07(1)`
- **Excursion #4 (AAVE/USD FLAT)**: `4` transitions across ticks [11015 .. 11123] | Net Return: `+0.89%`
  - Sequence: `R03(1) -> R01(1) -> R10(1) -> R03(1)`
- **Excursion #5 (NEAR/USD UP_FRICTION)**: `14` transitions across ticks [0 .. 206] | Net Return: `+0.18%`
  - Sequence: `R00(2) -> R08(1) -> R01(2) -> R07(1) -> R11(2) -> R07(2) -> R11(1) -> R03(1) -> R10(1) -> R06(1) -> R03(2) -> R10(1) -> R11(1) -> R03(1)`

![Excursion Dynamic Transitions Animation](plots/excursion_transitions.gif)

![Excursion Dynamic Transitions Summary](plots/excursion_transitions_summary.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `10` forming `7` sequential phases (enter: `1`, exit: `2`, wait: `4`)
- Balanced accuracy: `33.3%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `4/7` `57.1%`)
- Matthews Correlation Coefficient (MCC): `0.000`
- Enter action precision / recall: `0.0%` / `0.0%`
- Label-shuffled empirical null balanced accuracy: mean `33.3%`, 95th percentile `33.3%` (empirical p-value: `1.000`)
- Separates from null: `false`
- Post-teach memory retention: `7/7` (`100.0%`)
- Trie topology: `7` nodes, max depth `2`, mean depth `1.3`, branching factor `1.50`
- Basin geometry: records `6`, span `2`, active enter basins `1`, active exit basins `2` (total: `3`)
- Decisiveness: abstention rate `100.0%`, mean confidence `0.000`, mean contrast `0.000`
- Unseen background false-alarm rate: `5.89%` spurious triggers on continuous tape

#### Multi-Trie Empirical Comparison (Top-1 vs Top-2 vs Top-3)

| Configuration | Schema | Phases | Balanced Acc | Baseline Acc | MCC | Enter Prec / Rec | Retention | Nodes | Mean Depth | False Alarm | Null p-val |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Top-1 Winner (Rnn)** | `K=1` | 7 | `33.3%` | `33.3%` | `0.000` | `0.0%` / `0.0%` | `100.0%` | 7 | `1.3` | `5.89%` | `1.000` |
| **Top-2 Pair (Rnn-Rmm)** | `K=2` | 7 | `33.3%` | `33.3%` | `0.000` | `0.0%` / `0.0%` | `100.0%` | 9 | `1.7` | `0.00%` | `1.000` |
| **Top-3 Triad (Rnn-Rmm-Rpp)** | `K=3` | 7 | `25.0%` | `33.3%` | `0.000` | `0.0%` / `0.0%` | `85.7%` | 8 | `1.5` | `0.00%` | `1.000` |

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `21301`
- Tokens verified: `21283`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/var/folders/30/bdbpd0hj5wddgkxyx8pbc0mc0000gn/T/go-build4225469824/b001/exe/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `21301`
- Physical invariant violations: `3`
- Same-time ambiguous trades skipped: `10154`
- Comparisons per check: `map[cvd_coverage:11147 flow_aligned_midpoint_return:10920 level3_replay:5838487 midpoint_log_return:10920 signed_net_fraction_bounded:40838 trade_book:21279 trade_fields:21301]`
- Violations per check: `map[level3_replay:2 trade_book:1]`
- Unexercised checks: `[]`
- Verdict: `CONTRACT_BREACH`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `106510`
- Lookahead leakage detected: `false` (compared: `367349`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.035` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.036` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.018` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.007` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.087` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.002` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.042` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.036` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.006` bits (dominant: `false`)

