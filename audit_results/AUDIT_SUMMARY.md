# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791827121292573000` | **Symbol:** `ALL` | **Ticks:** `12914` | **Generated:** `2026-10-12T19:13:10Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **VALID** | declared hard domains on every observation; |z| <= rankBound(support); valid state & mature rank support | 0/362 domain breaches; 0 unstandardized mature; 0 z beyond bound; state pass=true |
| **0.5 Timing** | **VALID** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 0 spikes of 128996 |
| **1. Vitality** | **MEASURED** | descriptive | 42060 raw series, 42060 canonical cells, 662 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 4062305 pairs, 20.7% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.013 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 3645 emissions, H=2.944, p=0.020 |
| **5a. Ignition precursors** | **NOT_SUPPORTED** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.289 bits, p=0.255 |
| **5b. Exhaustion precursors** | **NOT_SUPPORTED** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.112 bits, p=0.118 |
| **5c. Held-out precursor skill** | **NOT_SUPPORTED** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 233 held-out, MCC 0.128, p=0.098 |
| **5d. Friction clearance** | **NOT_A_TEST** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **NOT_SUPPORTED** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 27.5% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **CONTRACT_BREACH** | cvd frames recomputed from the stored trade tape; every check exercised | 54 violations; unexercised [] |
| **V3. Causality** | **VALID** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 1426212 compared; changed: future 0, cross-symbol 0, epoch 0 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.009 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `362`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `0` observations beyond rank bound |z| <= rankBound(support) (max |z|=`3.39`, saturated=`1.3%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `128996`
- Ingestion latency (Timestamp - At): mean `-79.9ms`, p95 `-49.0ms`, max `98.2ms`
- Latency spikes: `0`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `VALID`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `42060` (varying `38565`, constant `597`, warming `2898`)
- Canonical grid cells: `42060` (varying `30224`, constant `662`, warming `11174`)
- High-correlation canonical pairs shown by the current reference filter: `62186`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|BONK/USD|overlap_density` | `96.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|FLUX/USD|relative_return_energy_velocity` | `77.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BONK/USD|midpoint_log_return` | `96.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|GNOT/USD|flow_aligned_midpoint_return` | `85.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|GNOT/USD|midpoint_response_per_net_notional` | `85.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|HNT/USD|signed_net_fraction_divergence` | `48.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|INJ/USD|signed_net_fraction_baseline` | `33.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PLUME/USD|aggressive_notional:buy` | `37.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PLUME/USD|executed_quantity:buy` | `37.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PLUME/USD|gross_executed_quantity` | `37.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PLUME/USD|gross_notional` | `37.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|PLUME/USD|mean_trade_notional` | `37.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|RAY/USD|aggressive_notional:buy` | `36.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|RAY/USD|executed_quantity:buy` | `36.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|XMR/USD|buy_notional_rate` | `65.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|XMR/USD|net_notional_rate` | `65.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|CC/USD|book_imbalance_velocity` | `94.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|CC/USD|resolution_gap_baseline` | `94.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|DRV/USD|book_imbalance` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|DRV/USD|book_notional:ask` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|DRV/USD|imbalance_resolution_gap` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|OP/USD|turnover_ratio` | `70.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|UNI/USD|turnover_zscore` | `32.7%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|XCN/USD|net_displayed_flow:ask` | `94.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|XCN/USD|net_displayed_flow_rate:ask` | `94.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `depthflow|XCN/USD|signed_net_displayed_flow_rate` | `94.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|background_rate` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|background_rate:buy` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|background_rate:sell` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|branching_spectral_radius` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|conditional_intensity` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|conditional_intensity:buy` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|conditional_intensity:sell` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_amplitude:buy_from_buy` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_amplitude:buy_from_sell` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_amplitude:sell_from_buy` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_amplitude:sell_from_sell` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_decay` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_fraction:buy` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_fraction:sell` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_intensity:buy` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_intensity:sell` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_mass:buy` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_mass:sell` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_share` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_share:buy` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_share:sell` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|excitation_timescale` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|expected_descendants_from_buy` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|APT/USD|expected_descendants_from_sell` | `43.9%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 612 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|AERO/USD|covariance_score_velocity` | `65.4%` | `94.1%` | `-0.0269` | `COLD_START` |
| `correlation|AERO/USD|relative_return_energy_velocity` | `65.4%` | `94.1%` | `-0.0087` | `COLD_START` |
| `correlation|AIOZ/USD|absolute_covariance_score` | `81.8%` | `88.9%` | `-0.0948` | `COLD_START` |
| `correlation|AIOZ/USD|cohort_absolute_covariance_score` | `81.8%` | `88.9%` | `0.1818` | `COLD_START` |
| `correlation|AIOZ/USD|cohort_covariance_score` | `81.8%` | `88.9%` | `0.1646` | `COLD_START` |
| `correlation|AIOZ/USD|cohort_covariance_score_dispersion` | `81.8%` | `88.9%` | `0.1818` | `COLD_START` |
| `correlation|AIOZ/USD|cohort_effective_peer_count` | `81.8%` | `88.9%` | `0.2113` | `COLD_START` |
| `correlation|AIOZ/USD|cohort_peer_count` | `81.8%` | `88.9%` | `0.2113` | `COLD_START` |
| `correlation|AIOZ/USD|covariance` | `81.8%` | `88.9%` | `-0.0537` | `COLD_START` |
| `correlation|AIOZ/USD|covariance_p_value` | `81.8%` | `88.9%` | `0.0948` | `COLD_START` |
| `correlation|AIOZ/USD|covariance_score` | `81.8%` | `88.9%` | `-0.0561` | `COLD_START` |
| `correlation|AIOZ/USD|covariance_score_baseline` | `81.8%` | `88.9%` | `0.0283` | `COLD_START` |
| `correlation|AIOZ/USD|covariance_score_divergence` | `81.8%` | `88.9%` | `0.1059` | `COLD_START` |
| `correlation|AIOZ/USD|covariance_score_zscore` | `81.8%` | `88.9%` | `0.1059` | `COLD_START` |
| `correlation|AIOZ/USD|covariance_standard_error` | `81.8%` | `88.9%` | `-0.1062` | `COLD_START` |
| ... and 3698 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `4062305`
- Direct relationships: `2092198`; inverse relationships: `1970107`
- Signed real mean: `0.017`; signed shuffled mean: `-0.000`
- 95th percentile of `|null r|`: `0.387`
- Real `|r|` above that empirical bound: `20.7%`
- KS distance in `|r|` space: `0.212`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid stationarity & completeness

- Cross-half excitation JSD: `0.013` bits (TVD `0.098`, p=`0.020`)
- Frames observed by the grid: `7419` of `12914` (`57.4%`)

![Grid Completeness](plots/stage3_grid_completeness.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `3645` across `12` regions
- Excitation strength: mean `0.100`, peak `1.001`, runner-up margin `0.026`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `21.9%`
- Real transition entropy: `2.944` bits
- Empirical dwell-block null mean: `3.249` bits
- Difference (null - real): `0.305` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `56` (chop, down, flat, up, up_friction)
- A->B Ignition: `NOT_SUPPORTED`, event/control `18/236`, JSD `0.289` vs null95 `0.395`
- B->C Exhaustion: `NOT_SUPPORTED`, event/control `260/135`, JSD `0.112` vs null95 `0.114`
- Supplemental non-excursion background observations: `6559`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `56.4%` | MCC: `0.128`
- Precision / Recall: `18.8%` / `18.8%`
- Mutual Information (Predictive Gain): `0.000` bits
- Prior Base Rate: `6.9%` | Top Precursor Tokens: `R08`

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `24` | Round-Trip Taker Fee: `160.00` bps
- Friction Clearance Rate: `50.0%` (`12` profitable / `12` unprofitable)
- Gross Mean Return: `1.59%` | Net Mean Return after Fees: `-0.02%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

#### 4. Excursion Dynamic Resonance: Regional Transitions

Captured `5` representative market excursion transition profiles. Consecutive identical winning region tokens are run-length deduplicated into distinct directional transitions:

- **Excursion #1 (CTR/USD UP)**: `5` transitions across ticks [0 .. 9996] | Net Return: `+4.10%`
  - Sequence: `R07(1) -> R09(1) -> R11(1) -> R06(1) -> R10(1)`
- **Excursion #2 (OSMO/USD UP)**: `9` transitions across ticks [483 .. 2402] | Net Return: `+3.37%`
  - Sequence: `R08(2) -> R07(2) -> R10(1) -> R00(1) -> R08(1) -> R09(2) -> R07(1) -> R09(1) -> R07(1)`
- **Excursion #3 (OGN/USD UP)**: `4` transitions across ticks [0 .. 3831] | Net Return: `+2.52%`
  - Sequence: `R09(1) -> R10(1) -> R11(1) -> R09(1)`
- **Excursion #4 (OGN/USD UP)**: `10` transitions across ticks [2137 .. 9447] | Net Return: `+2.30%`
  - Sequence: `R09(1) -> R10(1) -> R11(1) -> R09(2) -> R11(1) -> R09(3) -> R08(2) -> R05(1) -> R10(2) -> R04(1)`
- **Excursion #5 (ZRO/USD UP)**: `49` transitions across ticks [0 .. 8486] | Net Return: `+2.22%`
  - Sequence: `R10(3) -> R09(1) -> R10(2) -> R07(1) -> R09(1) -> R08(1) -> R10(1) -> R07(2) -> R06(2) -> R10(1) -> R09(1) -> R08(3) -> R10(1) -> R08(1) -> R03(1) -> R08(2) -> R10(1) -> R07(3) -> R08(1) -> R07(1) -> R11(1) -> R03(1) -> R11(2) -> R10(1) -> R11(9) -> R05(3) -> R07(2) -> R06(1) -> R11(3) -> R06(1) -> R04(2) -> R07(1) -> R04(1) -> R06(1) -> R03(1) -> R05(1) -> R07(1) -> R04(1) -> R06(2) -> R07(1) -> R04(1) -> R06(1) -> R10(1) -> R06(1) -> R00(1) -> R07(1) -> R10(3) -> R00(5) -> R04(1)`

![Excursion Dynamic Transitions Animation](plots/excursion_transitions.gif)

![Excursion Dynamic Transitions Summary](plots/excursion_transitions_summary.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `56` forming `29` sequential phases (enter: `3`, exit: `9`, wait: `17`)
- Balanced accuracy: `27.5%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `14/29` `48.3%`)
- Matthews Correlation Coefficient (MCC): `-0.136`
- Enter action precision / recall: `0.0%` / `0.0%`
- Label-shuffled empirical null balanced accuracy: mean `33.2%`, 95th percentile `44.0%` (empirical p-value: `0.941`)
- Separates from null: `false`
- Post-teach memory retention: `27/29` (`93.1%`)
- Trie topology: `294` nodes, max depth `98`, mean depth `28.0`, branching factor `1.07`
- Basin geometry: records `293`, span `98`, active enter basins `2`, active exit basins `9` (total: `11`)
- Decisiveness: abstention rate `82.8%`, mean confidence `0.172`, mean contrast `0.172`
- Unseen background false-alarm rate: `12.47%` spurious triggers on continuous tape

#### Multi-Trie Empirical Comparison (Top-1 vs Top-2 vs Top-3)

| Configuration | Schema | Phases | Balanced Acc | Baseline Acc | MCC | Enter Prec / Rec | Retention | Nodes | Mean Depth | False Alarm | Null p-val |
| :--- | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| **Top-1 Winner (Rnn)** | `K=1` | 29 | `27.5%` | `33.3%` | `-0.136` | `0.0%` / `0.0%` | `93.1%` | 294 | `28.0` | `12.47%` | `0.941` |
| **Top-2 Pair (Rnn-Rmm)** | `K=2` | 29 | `29.4%` | `33.3%` | `-0.092` | `0.0%` / `0.0%` | `96.6%` | 395 | `34.2` | `1.53%` | `0.941` |
| **Top-3 Triad (Rnn-Rmm-Rpp)** | `K=3` | 29 | `29.4%` | `33.3%` | `-0.092` | `0.0%` / `0.0%` | `96.6%` | 440 | `37.1` | `1.53%` | `0.961` |

> ⚠️ **LEARNING DEFICIT:** Prequential balanced accuracy (27.5%) trails baseline policy (33.3%). Memory retention is at 93.1%.

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `12914`
- Tokens verified: `7419`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/var/folders/30/bdbpd0hj5wddgkxyx8pbc0mc0000gn/T/go-build3503368110/b001/exe/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `12914`
- Physical invariant violations: `54`
- Same-time ambiguous trades skipped: `6234`
- Comparisons per check: `map[buy_notional_rate:6563 cvd_coverage:6680 level3_replay:2294080 midpoint_log_return:6569 rate_without_elapsed_time:1883 response_midpoint:at:6569 response_midpoint:from:5243 sell_notional_rate:6563 signed_net_fraction_bounded:6036 trade_book:12878 trade_fields:12914 trade_rate:6563]`
- Violations per check: `map[level3_replay:1 trade_book:53]`
- Unexercised checks: `[]`
- Verdict: `CONTRACT_BREACH`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `64570`
- Lookahead leakage detected: `false` (compared: `1426212`, contaminated: `0`)
- Cross-symbol contamination: `false`
- Epoch isolation passed: `true`
- Verdict: `VALID`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.188` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.006` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.016` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.027` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.005` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.005` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.007` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.063` bits (dominant: `false`)

