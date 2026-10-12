# SYMM Pipeline Empirical Audit

**State:** CONTRACT BREACHES PRESENT | **Epoch:** `1791786130376447000` | **Symbol:** `ALL` | **Ticks:** `17141` | **Generated:** `2026-10-12T07:05:20Z`

Only VALID and SUPPORTED are passes. MEASURED is descriptive, NOT_A_TEST cannot fail by construction, INSUFFICIENT_DATA means not evaluated.

Thresholds: significance `0.05`, latency spike `200ms` on at most `5%`, dominance `0.40` bits, duplication `0.45` bits.

| Stage | Verdict | Criterion | Evidence |
| :--- | :---: | :--- | :--- |
| **0. Contracts** | **VALID** | declared hard domains on every observation; |z| <= rankBound(support); valid state & mature rank support | 0/363 domain breaches; 0 unstandardized mature; 0 z beyond bound; state pass=true |
| **0.5 Timing** | **VALID** | no At regression per (source, symbol); spikes > 200ms on at most 5% | 0 inversions, 671 spikes of 193732 |
| **1. Vitality** | **MEASURED** | descriptive | 52109 raw series, 52109 canonical cells, 476 constant |
| **2. Sympathy** | **SUPPORTED** | within-symbol |r| exceedance vs block-shuffled null, p <= 0.05 | 5210580 pairs, 19.1% above null p95, p=0.020 |
| **3. Grid stationarity** | **MEASURED** | JSD between halves vs tick-shuffled null; stationary when p > 0.05 (ARI is NOT_A_TEST) | JSD 0.051 bits, p=0.020 |
| **4. Token dynamics** | **SUPPORTED** | transition entropy below block null, p <= 0.05 | 3048 emissions, H=2.244, p=0.020 |
| **5a. Ignition precursors** | **NOT_SUPPORTED** | event vs control excursions, excursion-label null, p <= 0.05 | JSD 0.040 bits, p=0.961 |
| **5b. Exhaustion precursors** | **NOT_SUPPORTED** | late vs early half within excursions, paired swap null, p <= 0.05 | JSD 0.148 bits, p=0.882 |
| **5c. Held-out precursor skill** | **INSUFFICIENT_DATA** | chronological 60/40 split, held-out MCC vs label null, p <= 0.05 | 1742 held-out, MCC 0.000, p=0.000 |
| **5d. Friction clearance** | **NOT_A_TEST** | detections are defined as moves that clear friction | restates the detector's own criterion |
| **6. Simulated trie** | **NOT_SUPPORTED** | in-memory trie (not S3) balanced accuracy vs label-shuffled null, p <= 0.05 | balanced accuracy 37.0% |
| **V1. Equivalence** | **NOT_A_TEST** | one code path compared with itself | deterministic=true |
| **V2. Truthfulness** | **CONTRACT_BREACH** | cvd frames recomputed from the stored trade tape; every check exercised | 455 violations; unexercised [] |
| **V3. Causality** | **CONTRACT_BREACH** | replayed z-scores/tokens unchanged by perturbed future, other symbols, prior epochs | 2186422 compared; changed: future 333002, cross-symbol 4735, epoch 329069 |
| **V4. Sensitivity** | **SUPPORTED** | no family moves regions > 0.40 bits on removal or > 0.45 on duplication (named limits) | 0 dominant; max duplication 0.011 bits |

---

### Stage 0: Declared mathematical contracts

- Series checked: `363`
- Hard-domain breaches: `0` series (`0` observations)
- Standardized z-score bound breaches: `0` observations beyond rank bound |z| <= rankBound(support) (max |z|=`3.49`, saturated=`1.6%`)
- Normalization breaches: `0`, Standardization breaches: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `193732`
- Ingestion latency (Timestamp - At): mean `-53.4ms`, p95 `-7.0ms`, max `420.0ms`
- Latency spikes: `671`
- Sequence inversions (timestamp regressions): `0`
- Feed status: `VALID`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `52109` (varying `48164`, constant `450`, warming `3495`)
- Canonical grid cells: `52109` (varying `37704`, constant `476`, warming `13929`)
- High-correlation canonical pairs shown by the current reference filter: `68121`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|NIL/USD|relative_return_energy` | `79.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `correlation|NIL/USD|return_energy_rate:measured` | `79.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|aggressive_notional:sell` | `89.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|executed_quantity:sell` | `89.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|sell_notional_rate` | `90.6%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|signed_count_fraction` | `89.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|signed_net_fraction` | `89.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|signed_net_fraction_baseline` | `87.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|signed_net_fraction_divergence` | `87.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|BLEND/USD|trade_count:sell` | `89.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|CPOOL/USD|sell_notional_rate` | `95.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cvd|KSM/USD|historical_path_percentile` | `37.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|background_rate` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|background_rate:buy` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|background_rate:sell` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|branching_spectral_radius` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|excitation_amplitude:buy_from_buy` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|excitation_amplitude:buy_from_sell` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|excitation_amplitude:sell_from_buy` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|excitation_amplitude:sell_from_sell` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|excitation_decay` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|excitation_timescale` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|expected_descendants_from_buy` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|expected_descendants_from_sell` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|offspring:buy_from_buy` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|offspring:buy_from_sell` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|offspring:sell_from_buy` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|AKT/USD|offspring:sell_from_sell` | `47.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|BLEND/USD|arrival_rate:sell` | `92.2%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|BLEND/USD|event_count:sell` | `93.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|BLEND/USD|event_fraction:buy` | `93.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|BLEND/USD|event_fraction:sell` | `93.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|BLZ/USD|event_count:buy` | `94.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|BLZ/USD|event_fraction:buy` | `94.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|BLZ/USD|event_fraction:sell` | `94.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|CPOOL/USD|event_count:sell` | `100.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|ELX/USD|event_count:sell` | `89.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|ELX/USD|event_fraction:buy` | `89.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|ELX/USD|event_fraction:sell` | `89.5%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|background_rate` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|background_rate:buy` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|background_rate:sell` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|branching_spectral_radius` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|excitation_amplitude:buy_from_buy` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|excitation_amplitude:buy_from_sell` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|excitation_amplitude:sell_from_buy` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|excitation_amplitude:sell_from_sell` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|excitation_decay` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|excitation_timescale` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `hawkes|JTO/USD|expected_descendants_from_buy` | `56.8%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| ... and 426 more dead cells | | | | |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `correlation|ALIGN/USD|overlap_density` | `56.5%` | `84.6%` | `-0.1492` | `COLD_START` |
| `correlation|ALTHEA/USD|absolute_covariance_score` | `36.8%` | `85.7%` | `-0.0433` | `COLD_START` |
| `correlation|ALTHEA/USD|cohort_absolute_covariance_score` | `36.8%` | `85.7%` | `-0.0433` | `COLD_START` |
| `correlation|ALTHEA/USD|cohort_covariance_score` | `36.8%` | `85.7%` | `0.0615` | `COLD_START` |
| `correlation|ALTHEA/USD|cohort_covariance_score_dispersion` | `36.8%` | `85.7%` | `-0.1525` | `COLD_START` |
| `correlation|ALTHEA/USD|cohort_effective_peer_count` | `36.8%` | `85.7%` | `-0.0085` | `COLD_START` |
| `correlation|ALTHEA/USD|cohort_peer_count` | `36.8%` | `85.7%` | `0.1525` | `COLD_START` |
| `correlation|ALTHEA/USD|covariance` | `36.8%` | `85.7%` | `-0.1525` | `COLD_START` |
| `correlation|ALTHEA/USD|covariance_p_value` | `36.8%` | `85.7%` | `0.0433` | `COLD_START` |
| `correlation|ALTHEA/USD|covariance_score` | `36.8%` | `85.7%` | `-0.0808` | `COLD_START` |
| `correlation|ALTHEA/USD|covariance_score_baseline` | `36.8%` | `85.7%` | `-0.2093` | `COLD_START` |
| `correlation|ALTHEA/USD|covariance_score_divergence` | `36.8%` | `85.7%` | `0.0710` | `COLD_START` |
| `correlation|ALTHEA/USD|covariance_score_zscore` | `36.8%` | `85.7%` | `0.0911` | `COLD_START` |
| `correlation|ALTHEA/USD|covariance_standard_error` | `36.8%` | `85.7%` | `0.1018` | `COLD_START` |
| `correlation|ALTHEA/USD|historical_path_distance` | `36.8%` | `85.7%` | `-0.2093` | `COLD_START` |
| ... and 3377 more stagnant cells | | | | |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `5210580`
- Direct relationships: `2686685`; inverse relationships: `2523895`
- Signed real mean: `0.017`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.398`
- Real `|r|` above that empirical bound: `19.1%`
- KS distance in `|r|` space: `0.197`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid reproducibility

- Early grid: `363` cells / `12` regions
- Late grid: `362` cells / `12` regions
- Shared universe: `362` cells (`99.7%`)
- Adjusted Rand Index: `1.000`

> Balanced region sizes are enforced by the partitioner and are not presented as empirical evidence. No ARI health cutoff is applied.

![Stage 3](plots/stage3_region_partitioning.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `3048` across `12` regions
- Excitation strength: mean `4.108`, peak `22.202`, runner-up margin `1.506`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `41.4%`
- Real transition entropy: `2.244` bits
- Empirical dwell-block null mean: `2.765` bits
- Difference (null - real): `0.521` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `1397` (chop, down, flat, up, up_friction)
- A->B Ignition: `NOT_SUPPORTED`, event/control `127/2568`, JSD `0.040` vs null95 `0.306`
- B->C Exhaustion: `NOT_SUPPORTED`, event/control `89/190`, JSD `0.148` vs null95 `0.218`
- Supplemental non-excursion background observations: `2704`

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

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `1397` forming `511` sequential phases (enter: `5`, exit: `7`, wait: `499`)
- Balanced accuracy: `37.0%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `484/511` `94.7%`)
- Matthews Correlation Coefficient (MCC): `-0.004`
- Enter action precision / recall: `0.0%` / `0.0%`
- Label-shuffled empirical null balanced accuracy: mean `33.2%`, 95th percentile `41.9%` (empirical p-value: `0.255`)
- Separates from null: `false`
- Post-teach memory retention: `475/511` (`93.0%`)
- Trie topology: `768` nodes, max depth `86`, mean depth `15.5`, branching factor `1.18`
- Basin geometry: records `767`, span `86`, active enter basins `5`, active exit basins `7` (total: `12`)
- Decisiveness: abstention rate `96.7%`, mean confidence `0.008`, mean contrast `0.207`
- Unseen background false-alarm rate: `1.44%` spurious triggers on continuous tape

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `17141`
- Tokens verified: `5552`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/Users/theapemachine/Library/Caches/go-build/3c/3cedecfff56fda1307845eea5de3366c6bebe6c05f1ca096cb36a23f9f1c5732-d/main`
- Verdict: `NOT_A_TEST` (one code path compared with itself)

### Validation 2: Metric Truthfulness

- Measurements audited: `19380`
- Physical invariant violations: `455`
- Same-time ambiguous trades skipped: `9536`
- Comparisons per check: `map[buy_notional_rate:9699 cvd_coverage:9844 level3_replay:4266623 midpoint_log_return:9717 rate_without_elapsed_time:2974 response_midpoint:at:9717 response_midpoint:from:7637 sell_notional_rate:9699 signed_net_fraction_bounded:9335 trade_book:19363 trade_fields:19380 trade_rate:9699]`
- Violations per check: `map[level3_replay:1 trade_book:454]`
- Unexercised checks: `[]`
- Verdict: `CONTRACT_BREACH`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `97922`
- Lookahead leakage detected: `true` (compared: `2186422`, contaminated: `333002`)
- Cross-symbol contamination: `true`
- Epoch isolation passed: `false`
- Verdict: `CONTRACT_BREACH`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Verdict: `SUPPORTED`

  - Family `correlation`: Removed JSD = `0.175` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.039` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.005` bits (dominant: `false`)
  - Family `morphology`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.010` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.002` bits (dominant: `false`)

