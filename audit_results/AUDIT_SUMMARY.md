# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791928500389570000` | **Symbol:** `ALL` | **Ticks:** `39931` | **Generated:** `2026-10-13T23:51:12Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **CONTRACT_BREACH** | declared hard domains on every observation; bounded SNM in [-1, 1]; valid state | 0/43 domain breaches; 0 norm breaches; state pass=true |
| **0.5 Timing** | **VALID** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 2920 spikes of 399246 |
| **1. Vitality** | **MEASURED** | descriptive | 14450 raw series, 14450 canonical cells, 1427 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 127060 pairs, 11.9% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.012 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 15973 emissions, H=2.803, p=0.020 |
| **5a. Ignition precursors** | **NOT_SUPPORTED** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.027 bits, p=0.961 |
| **5b. Exhaustion precursors** | **NOT_SUPPORTED** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.105 bits, p=0.686 |
| **5c. Held-out precursor skill** | **NOT_SUPPORTED** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 95 held-out, MCC -0.110, p=0.824 |
| **5d. Friction clearance** | **NOT_A_TEST** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **NOT_SUPPORTED** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 29.2% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **CONTRACT_BREACH** | cvd frames recomputed from the stored trade tape; every check exercised | 59 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 645055 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.008 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `43`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `0` observations beyond rank bound |z| <= rankBound(support) (max |z|=`1.00`, saturated=`3.9%`)
- Normalization breaches: `0`, Standardization breaches: `52581`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `399246`
- Ingestion latency (Timestamp - At): mean `-26.3ms`, p95 `34.2ms`, max `1333.6ms`
- Latency spikes: `2920`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `VALID`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `14450` (varying `11722`, constant `1037`, warming `1152`)
- Canonical grid cells: `14450` (varying `11332`, constant `1427`, warming `1152`)
- High-correlation canonical pairs shown by the current reference filter: `428`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|BLUAI/USD|cohort_covariance_score` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|CHEX/USD|cohort_covariance_score` | `60.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|DRIFT/USD|cohort_covariance_score` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|IDOS/USD|cohort_covariance_score` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|INIT/USD|cohort_covariance_score` | `87.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|KULA/USD|cohort_covariance_score` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|LQTY/USD|cohort_covariance_score` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|NANO/USD|cohort_covariance_score` | `25.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|STG/USD|cohort_covariance_score` | `27.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ACU/USD|flow_aligned_midpoint_return` | `87.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ACU/USD|midpoint_log_return` | `87.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ALKIMI/USD|flow_aligned_midpoint_return` | `83.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ALKIMI/USD|midpoint_log_return` | `83.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ALLO/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ALLO/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ARKM/USD|signed_net_fraction` | `22.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|AVNT/USD|signed_net_fraction` | `37.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BADGER/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BADGER/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|signed_net_fraction` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLUAI/USD|signed_net_fraction` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BNT/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BOB/USD|flow_aligned_midpoint_return` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BOB/USD|midpoint_log_return` | `75.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BRL1/USD|flow_aligned_midpoint_return` | `91.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BRL1/USD|midpoint_log_return` | `91.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CLANKER/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CLANKER/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|COQ/USD|signed_net_fraction` | `28.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DEBIT/USD|signed_net_fraction` | `33.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DEEP/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DEEP/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DEGEN/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|DEGEN/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|ETHFI/USD|signed_net_fraction` | `70.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|FLOKI/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|FLOKI/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|G/USD|signed_net_fraction` | `40.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|GHST/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|GWEI/USD|signed_net_fraction` | `50.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|IDOS/USD|flow_aligned_midpoint_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|IDOS/USD|midpoint_log_return` | `80.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|INIT/USD|signed_net_fraction` | `62.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|JOE/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|JOE/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KAT/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KAT/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LPT/USD|flow_aligned_midpoint_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LPT/USD|midpoint_log_return` | `66.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|LSK/USD|signed_net_fraction` | `57.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 1377 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|CVX/USD|cohort_covariance_score` | `83.3%` | `80.0%` | `-0.0596` | `COLD_START` |
| `correlation|INIT/USD|historical_path_percentile` | `87.5%` | `92.9%` | `-0.0345` | `COLD_START` |
| `correlation|YGG/USD|cohort_covariance_score` | `68.8%` | `81.8%` | `-0.0144` | `COLD_START` |
| `cvd|AKT/USD|signed_net_fraction` | `50.0%` | `87.5%` | `-0.0292` | `COLD_START` |
| `cvd|ASSET/USD|flow_aligned_midpoint_return` | `83.3%` | `80.0%` | `-0.1000` | `COLD_START` |
| `cvd|ASSET/USD|midpoint_log_return` | `83.3%` | `80.0%` | `0.1000` | `COLD_START` |
| `cvd|AZTEC/USD|signed_net_fraction` | `61.2%` | `88.7%` | `0.0014` | `COLD_START` |
| `cvd|BIO/USD|signed_net_fraction` | `41.7%` | `80.0%` | `0.0293` | `COLD_START` |
| `cvd|BLUR/USD|signed_net_fraction` | `38.4%` | `82.1%` | `-0.0068` | `COLD_START` |
| `cvd|BOS/USD|signed_net_fraction` | `62.5%` | `80.0%` | `0.1333` | `COLD_START` |
| `cvd|BTC/USD|flow_aligned_midpoint_return` | `100.0%` | `80.4%` | `0.0376` | `COLD_START` |
| `cvd|BTC/USD|midpoint_log_return` | `100.0%` | `80.4%` | `-0.0022` | `COLD_START` |
| `cvd|CPOOL/USD|signed_net_fraction` | `81.6%` | `87.0%` | `-0.0030` | `COLD_START` |
| `cvd|CVX/USD|signed_net_fraction` | `27.8%` | `80.0%` | `0.1333` | `COLD_START` |
| `cvd|FORTH/USD|signed_net_fraction` | `50.0%` | `83.3%` | `0.1175` | `COLD_START` |
| ... and 685 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `127060`
- Direct relationships: `63605`; inverse relationships: `63381`
- Signed real mean: `0.005`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.396`
- Real `|r|` above that empirical bound: `11.9%`
- KS distance in `|r|` space: `0.121`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid stationarity & completeness

- Cross-half excitation JSD: `0.012` bits (TVD `0.088`, p=`0.020`)
- Frames observed by the grid: `39922` of `39931` (`100.0%`)

![Grid Completeness](plots/stage3_grid_completeness.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `15973` across `12` regions
- Excitation strength: mean `1.042`, peak `5.018`, runner-up margin `0.283`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `24.3%`
- Real transition entropy: `2.803` bits
- Empirical dwell-block null mean: `2.914` bits
- Difference (null - real): `0.112` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `25` (chop, down, flat, up, up_friction)
- A->B Ignition: `NOT_SUPPORTED`, event/control `60/137`, JSD `0.027` vs null95 `0.227`
- B->C Exhaustion: `NOT_SUPPORTED`, event/control `104/82`, JSD `0.105` vs null95 `0.149`
- Supplemental non-excursion background observations: `39230`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `41.5%` | MCC: `-0.110`
- Precision / Recall: `8.1%` / `50.0%`
- Mutual Information (Predictive Gain): `0.008` bits
- Prior Base Rate: `10.5%` | Top Precursor Tokens: `R01, R02, R03, R04, R05, R08`

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `10` | Round-Trip Taker Fee: `160.00` bps
- Friction Clearance Rate: `50.0%` (`5` profitable / `5` unprofitable)
- Gross Mean Return: `2.92%` | Net Mean Return after Fees: `1.29%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

#### 4. Excursion Dynamic Resonance: Regional Transitions

Captured `5` representative market excursion transition profiles. Consecutive identical winning region tokens are run-length deduplicated into distinct directional transitions:

- **Excursion #1 (SWEAT/USD UP)**: `45` transitions across ticks [5585 .. 6485] | Net Return: `+13.33%`
  - Sequence: `R05(1) -> R08(1) -> R11(1) -> R10(1) -> R05(1) -> R10(1) -> R03(3) -> R10(1) -> R11(1) -> R08(1) -> R01(1) -> R04(1) -> R01(3) -> R03(1) -> R01(1) -> R03(3) -> R08(1) -> R01(1) -> R03(1) -> R05(1) -> R03(3) -> R10(1) -> R03(1) -> R10(1) -> R04(1) -> R10(1) -> R03(1) -> R08(1) -> R01(1) -> R05(1) -> R10(2) -> R11(6) -> R00(1) -> R11(1) -> R00(1) -> R11(6) -> R10(1) -> R03(3) -> R10(1) -> R03(1) -> R05(1) -> R03(2) -> R01(1) -> R11(1) -> R03(1)`
- **Excursion #2 (ZEUS/USD UP)**: `30` transitions across ticks [0 .. 3205] | Net Return: `+6.77%`
  - Sequence: `R00(1) -> R08(1) -> R03(1) -> R08(1) -> R10(1) -> R03(1) -> R01(1) -> R03(1) -> R10(1) -> R01(1) -> R09(1) -> R01(3) -> R03(2) -> R11(1) -> R03(1) -> R07(1) -> R11(1) -> R03(2) -> R01(1) -> R09(1) -> R11(1) -> R03(1) -> R01(1) -> R03(1) -> R04(1) -> R01(1) -> R03(3) -> R01(1) -> R11(1) -> R03(3)`
- **Excursion #3 (BAT/USD UP)**: `67` transitions across ticks [0 .. 3354] | Net Return: `+3.32%`
  - Sequence: `R00(1) -> R07(2) -> R03(1) -> R07(1) -> R03(1) -> R02(1) -> R03(1) -> R11(1) -> R07(1) -> R11(1) -> R03(1) -> R09(1) -> R11(1) -> R05(1) -> R01(1) -> R03(5) -> R01(3) -> R03(1) -> R07(1) -> R01(1) -> R03(1) -> R01(1) -> R03(1) -> R07(1) -> R03(1) -> R06(2) -> R10(1) -> R04(1) -> R03(1) -> R08(1) -> R05(1) -> R03(1) -> R10(1) -> R09(1) -> R01(1) -> R03(1) -> R08(1) -> R03(1) -> R01(1) -> R11(1) -> R01(1) -> R10(1) -> R01(1) -> R05(1) -> R01(1) -> R08(1) -> R03(4) -> R08(1) -> R03(1) -> R10(1) -> R01(2) -> R03(5) -> R01(1) -> R10(1) -> R01(2) -> R07(1) -> R01(1) -> R03(3) -> R05(1) -> R09(1) -> R01(1) -> R03(2) -> R01(1) -> R10(1) -> R01(1) -> R03(1) -> R01(1)`
- **Excursion #4 (KTA/USD UP)**: `26` transitions across ticks [0 .. 6433] | Net Return: `+2.57%`
  - Sequence: `R00(1) -> R07(2) -> R03(1) -> R10(1) -> R07(1) -> R03(1) -> R01(1) -> R05(1) -> R10(1) -> R03(2) -> R10(1) -> R11(1) -> R03(4) -> R01(1) -> R10(1) -> R01(1) -> R03(3) -> R01(1) -> R10(1) -> R01(1) -> R08(1) -> R01(1) -> R09(1) -> R01(1) -> R03(2) -> R01(1)`
- **Excursion #5 (APU/USD UP)**: `14` transitions across ticks [12968 .. 13924] | Net Return: `+2.15%`
  - Sequence: `R04(1) -> R03(3) -> R01(1) -> R09(1) -> R11(1) -> R09(1) -> R11(2) -> R08(1) -> R03(2) -> R09(1) -> R05(1) -> R03(3) -> R10(1) -> R08(1)`

![Excursion Dynamic Transitions Animation](plots/excursion_transitions.gif)

![Excursion Dynamic Transitions Summary](plots/excursion_transitions_summary.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `25` forming `25` sequential phases (enter: `4`, exit: `5`, wait: `16`)
- Balanced accuracy: `29.2%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `14/25` `56.0%`)
- Matthews Correlation Coefficient (MCC): `-0.089`
- Enter action precision / recall: `0.0%` / `0.0%`
- Label-shuffled empirical null balanced accuracy: mean `33.1%`, 95th percentile `41.7%` (empirical p-value: `0.902`)
- Separates from null: `false`
- Post-teach memory retention: `22/25` (`88.0%`)
- Trie topology: `29` nodes, max depth `9`, mean depth `3.0`, branching factor `1.40`
- Basin geometry: records `28`, span `9`, active enter basins `4`, active exit basins `5` (total: `9`)
- Decisiveness: abstention rate `92.0%`, mean confidence `0.080`, mean contrast `0.080`
- Unseen background false-alarm rate: `18.67%` spurious triggers on continuous tape

#### Multi-Trie Empirical Comparison (Top-1 vs Top-2 vs Top-3)

| Configuration | Schema | Phases | Balanced Acc | Baseline Acc | MCC | Enter Prec / Rec | Retention | Nodes | Mean Depth | False Alarm | Null p-val |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Top-1 Winner (Rnn)** | `K=1` | 25 | `29.2%` | `33.3%` | `-0.089` | `0.0%` / `0.0%` | `88.0%` | 29 | `3.0` | `18.67%` | `0.902` |
| **Top-2 Pair (Rnn-Rmm)** | `K=2` | 25 | `33.3%` | `33.3%` | `0.055` | `20.0%` / `25.0%` | `80.0%` | 16 | `2.1` | `0.00%` | `0.314` |
| **Top-3 Triad (Rnn-Rmm-Rpp)** | `K=3` | 25 | `22.9%` | `33.3%` | `-0.190` | `0.0%` / `0.0%` | `72.0%` | 15 | `1.7` | `0.00%` | `0.922` |

> ⚠️ **LEARNING DEFICIT:** Prequential balanced accuracy (29.2%) trails baseline policy (33.3%). Memory retention is at 88.0%.

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `39931`
- Tokens verified: `39922`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/Users/theapemachine/Library/Caches/go-build/69/692dac1ed134c544dcdecb2b4f1e8aabbb48a28641edb34f76d934314384d07c-d/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `39931`
- Physical invariant violations: `59`
- Same-time ambiguous trades skipped: `17605`
- Comparisons per check: `map[cvd_coverage:22326 flow_aligned_midpoint_return:22023 level3_replay:13733109 midpoint_log_return:22023 signed_net_fraction_bounded:38310 trade_book:39912 trade_fields:39931]`
- Violations per check: `map[level3_replay:7 trade_book:52]`
- Unexercised checks: `[]`
- Verdict: `CONTRACT_BREACH`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `199660`
- Lookahead leakage detected: `false` (compared: `645055`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.033` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.055` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.021` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.008` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.068` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.002` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.035` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.021` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.002` bits (dominant: `false`)

