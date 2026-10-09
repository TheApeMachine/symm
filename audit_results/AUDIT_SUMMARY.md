# SYMM Pipeline Empirical Audit

**State:** CONTRACT_BREACHES PRESENT | **Epoch:** `1791494698631035000` | **Symbol:** `ALL` | **Ticks:** `1000` | **Generated:** `2026-10-09T00:53:01Z`

This report follows [the empirical audit contract](../hindsight/audit/AUDIT_CONTRACT.md): hard mathematical contracts may fail; descriptive stages report measurements; missing evidence is explicit.

| Stage | Question | Experiment state | Observation |
| :--- | :--- | :---: | :--- |
| **0. Contracts** | Do declared hard domains hold? | **CONTRACT_BREACH** | 869/6788 series breached (21015 observations) |
| **0.5 Timing** | Is ingestion clock synchronized and monotonic? | **MEASURED** | Mean drift -65.9ms (p95 34.7ms); 68 spikes; 564 sequence inversions |
| **1. Vitality** | What raw/canonical evidence actually exists? | **MEASURED** | 6788 raw series; 365 canonical cells; 12 constant canonical cells |
| **2. Sympathy** | Do observed deformations relate beyond a mask-preserving shuffled null? | **MEASURED** | 62128 pairs; |null| p95 0.101; 35.8% real |r| above it; KS 0.329 |
| **3. Grid reproducibility** | Do disjoint periods recover the same co-memberships? | **MEASURED** | ARI 1.000; 100.0% universe overlap (365 shared cells) |
| **4. Token dynamics** | What does a frozen grid emit on unseen tape? | **MEASURED** | 494 emissions; 20 regions; H=3.173 vs null mean 3.268 |
| **5. Precursors** | Are A->B and B->C populations measurable? | **A->B MEASURED / B->C MEASURED** | A->B N=147/448, JSD 0.050 vs null95 0.055; B->C N=196/198 |
| **6. Cognitive Trie** | Does prequential recall beat baselines and retain memory? | **MEASURED** | Balanced Acc 37.5% (vs baseline 33.3%, null95 38.4%); MCC 0.066; Enter Prec/Rec 16.0%/56.5%; retention 45.2% |

---

### Stage 0: Declared mathematical contracts

- Series checked: `6788`
- Series with hard-domain breaches: `869`
- Breach observations: `21015`

| Metric | Unit | Declared domain | Observed range | Breaches |
| :--- | :---: | :---: | :---: | ---: |
| `log_likelihood:poisson` | `nat` | `[0, +inf)` | `[-6449.667, 157.949]` | 1502/1538 |
| `log_likelihood:self_only` | `nat` | `[0, +inf)` | `[-6457.397, 248.547]` | 1415/1538 |
| `log_likelihood:hawkes` | `nat` | `[0, +inf)` | `[-6457.397, 248.547]` | 1386/1538 |
| `log_likelihood_per_event:hawkes` | `nat` | `[0, +inf)` | `[-99.345, 3.824]` | 1386/1538 |
| `count_innovation:sell` | `count` | `[0, +inf)` | `[-768.932, 40.456]` | 794/1538 |
| `count_innovation:buy` | `count` | `[0, +inf)` | `[-6469.306, 38.827]` | 777/1538 |
| `log_likelihood_gain_vs_self_only` | `nat` | `[0, +inf)` | `[-16.421, 213.533]` | 553/1538 |
| `log_likelihood_gain_per_event_vs_self_only` | `nat` | `[0, +inf)` | `[-0.253, 3.285]` | 553/1538 |
| `best_lag_correlation@MIM/USD` | `correlation` | `[-1, 1]` | `[-1.721, 1.788]` | 220/1078 |
| `contemporaneous_correlation@MIM/USD` | `correlation` | `[-1, 1]` | `[-1.721, 1.785]` | 204/1078 |
| `absolute_correlation_gain@SIGN/USD` | `correlation` | `[0, 1]` | `[-0.202, 0.197]` | 8/44 |
| `absolute_correlation_gain@HFT/USD` | `correlation` | `[0, 1]` | `[-0.196, 0.617]` | 35/228 |

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `11600`
- Ingestion latency (Timestamp - At): mean `-65.9ms`, p95 `34.7ms`, max `1037.1ms`
- Latency spikes (>200ms or 5x median): `68`
- Sequence inversions (timestamp regressions): `564`
- Feed status: `MEASURED`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `6788` (varying `6047`, constant `741`)
- Canonical grid cells: `365` (varying `353`, constant `12`)
- High-correlation canonical pairs shown by the current reference filter: `406`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `asof_age_seconds` | `80.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `cohort_member_count` | `80.3%` | `0.0%` | `[571.000, 571.000]` | `DEAD` |
| `effective_sample_count` | `76.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `excluded_member_count` | `80.3%` | `0.0%` | `[41.000, 41.000]` | `DEAD` |
| `focal_return_energy_rate` | `76.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `largest_move_tie_count` | `80.3%` | `0.0%` | `[1.000, 1.000]` | `DEAD` |
| `median_return` | `80.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `median_return_velocity` | `80.3%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `reference_symbol` | `67.5%` | `0.0%` | `[1.000, 1.000]` | `DEAD` |
| `relative_cohort_return_energy` | `76.1%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `valid_member_count` | `80.3%` | `0.0%` | `[530.000, 530.000]` | `DEAD` |
| `volume_bar_target_quantity` | `89.3%` | `0.0%` | `[1.000, 1.000]` | `DEAD` |

#### Stagnant / Cold-Start Canonical Cells (>=80% Zero)

| Canonical Cell | Coverage | Zero Fraction | Mean | Status |
| :--- | :---: | :---: | :---: | :---: |
| `negative_midpoint_return` | `89.3%` | `90.6%` | `0.0001` | `COLD_START` |
| `net_replenishment_rate:ask` | `34.1%` | `82.7%` | `575.2334` | `COLD_START` |
| `net_replenishment_rate:bid` | `34.1%` | `81.5%` | `17924.6117` | `COLD_START` |
| `net_withdrawal_fraction:ask` | `34.1%` | `90.0%` | `0.0337` | `COLD_START` |
| `net_withdrawal_fraction:bid` | `34.1%` | `88.3%` | `0.0352` | `COLD_START` |
| `net_withdrawal_rate:ask` | `34.1%` | `90.9%` | `43027.2661` | `COLD_START` |
| `net_withdrawal_rate:bid` | `34.1%` | `90.6%` | `656.0260` | `COLD_START` |
| `net_withdrawn_quantity:ask` | `34.1%` | `90.0%` | `929.5649` | `COLD_START` |
| `net_withdrawn_quantity:bid` | `34.1%` | `88.3%` | `69.0384` | `COLD_START` |
| `positive_midpoint_return` | `89.3%` | `90.8%` | `0.0001` | `COLD_START` |
| `withdrawal_fraction_velocity:ask` | `34.1%` | `84.5%` | `0.7172` | `COLD_START` |
| `withdrawal_fraction_velocity:bid` | `34.1%` | `84.2%` | `-2.8688` | `COLD_START` |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `62128`
- Direct relationships: `31438`; inverse relationships: `30690`
- Signed real mean: `0.011`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.101`
- Real `|r|` above that empirical bound: `35.8%`
- KS distance in `|r|` space: `0.329`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid reproducibility

- Early grid: `365` cells / `20` regions
- Late grid: `365` cells / `20` regions
- Shared universe: `365` cells (`100.0%`)
- Adjusted Rand Index: `1.000`

> Balanced region sizes are enforced by the partitioner and are not presented as empirical evidence. No ARI health cutoff is applied.

![Stage 3](plots/stage3_region_partitioning.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `494` across `20` regions
- Excitation strength: mean `0.065`, peak `1.705`, runner-up margin `0.032`
- Active cell coverage: `53.5%` mean
- Maximum observed token share: `14.8%`
- Real transition entropy: `3.173` bits
- Empirical dwell-block null mean: `3.268` bits
- Difference (null - real): `0.095` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

- Detections: `289` (chop, down, flat, up, up_friction)
- A->B: `MEASURED`, event/control `147/448`, JSD `0.050`, null95 `0.055`
- B->C: `MEASURED`, event/control `196/198`, JSD `0.066`, null95 `0.072`
- Supplemental non-excursion background observations: `133`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `289` forming `168` sequential phases (enter: `23`, exit: `65`, wait: `80`)
- Balanced accuracy: `37.5%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `50/168` `29.8%`)
- Matthews Correlation Coefficient (MCC): `0.066`
- Enter action precision / recall: `16.0%` / `56.5%`
- Label-shuffled empirical null balanced accuracy: mean `32.7%`, 95th percentile `38.4%` (empirical p-value: `0.580`)
- Separates from null: `false`
- Post-teach memory retention: `76/168` (`45.2%`)
- Trie topology: `1205` nodes, max depth `5`, mean depth `1.8`, branching factor `17.20`
- Basin geometry: records `2081`, span `4`, active enter basins `616`, active exit basins `519` (total: `1135`)
- Decisiveness: abstention rate `5.4%`, mean confidence `0.549`, mean contrast `0.816`
- Unseen background false-alarm rate: `105.75%` spurious triggers on continuous tape

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

