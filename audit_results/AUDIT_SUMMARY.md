# SYMM Pipeline Empirical Audit

**State:** CONTRACT_BREACHES PRESENT | **Epoch:** `1791596128450467000` | **Symbol:** `ALL` | **Ticks:** `3665` | **Generated:** `2026-10-10T01:43:46Z`

This report follows [the empirical audit contract](../hindsight/audit/AUDIT_CONTRACT.md): hard mathematical contracts may fail; descriptive stages report measurements; missing evidence is explicit.

| Stage | Question | Experiment state | Observation |
| :--- | :--- | :---: | :--- |
| **0. Contracts** | Do declared hard domains hold? | **CONTRACT_BREACH** | 2/327 series breached (443 observations) |
| **0.5 Timing** | Is ingestion clock synchronized and monotonic? | **MEASURED** | Mean drift -6.1ms (p95 64.0ms); 36 spikes; 1592 sequence inversions |
| **1. Vitality** | What raw/canonical evidence actually exists? | **MEASURED** | 327 raw series; 352 canonical cells; 7 constant canonical cells |
| **2. Sympathy** | Do observed deformations relate beyond a mask-preserving shuffled null? | **MEASURED** | 59313 pairs; |null| p95 0.096; 9.4% real |r| above it; KS 0.080 |
| **3. Grid reproducibility** | Do disjoint periods recover the same co-memberships and stationary excitation? | **MEASURED** | ARI 1.000 (deterministic); Overlap 100.0%; JSD 0.021 bits (stationary: true) |
| **4. Token dynamics** | What does a frozen grid emit on unseen tape? | **MEASURED** | 1561 raw (1038 compressed, stay 36.8%); Raw H=2.568; Comp H=2.724 |
| **5. Precursors** | Statistical separation, predictive skill & economic friction clearance | **MEASURED** | JSD 0.027b; BalAcc 55.1% (MCC 0.039); Friction Clearance 18.1% (N=94) |
| **6. Cognitive Trie & S3 Memory** | Does associative memory disambiguate and beat baselines? | **MEASURED** | Balanced Acc 30.4% (vs baseline 33.3%); S3 keys 265, collisions 5, disambiguation 3 tokens |
| **V1. Equivalence** | Does audit execution match production paths bit-for-bit? | **true** | Mismatches: tokens=0, metrics=0 across 3665 ticks |
| **V2. Truthfulness** | Do published metrics truthfully reflect raw tape events? | **true** | Violations: 0, zero-filled midpoints=0, synthetic time=0 |
| **V3. Causality** | Are emissions causally isolated from future and other symbols? | **false** | Future leakage=true, cross-symbol contamination=true |
| **V4. Sensitivity** | Does grid resist dominance and retain balanced confluence? | **true** | Families tested: 10, duplication-resistant=true |

---

### Stage 0: Declared mathematical contracts

- Series checked: `327`
- Series with hard-domain breaches: `2`
- Breach observations: `443`

| Metric | Unit | Declared domain | Observed range | Breaches |
| :--- | :---: | :---: | :---: | ---: |
| `excitation_mass:sell` | `count` | `[0, +inf)` | `[-0.000, 22.724]` | 223/1977 |
| `excitation_mass:buy` | `count` | `[0, +inf)` | `[-0.000, 19.456]` | 220/1977 |

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 0.5: Ingestion clock timing & synchronization

- Observations checked: `37516`
- Ingestion latency (Timestamp - At): mean `-6.1ms`, p95 `64.0ms`, max `1283.4ms`
- Latency spikes (>200ms or 5x median): `36`
- Sequence inversions (timestamp regressions): `1592`
- Feed status: `MEASURED`

> Drift measures local ingest latency relative to venue event time. Sequence inversions indicate out-of-order ingress.

### Stage 1: Observed metric population

- Raw named series: `327` (varying `320`, constant `7`)
- Canonical grid cells: `352` (varying `345`, constant `7`)
- High-correlation canonical pairs shown by the current reference filter: `302`

#### Constant / Dead Canonical Grid Cells

| Canonical Cell | Coverage | Zero Fraction | Range | Status |
| :--- | :---: | :---: | :---: | :---: |
| `sentiment:directional_agreement` | `3.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `sentiment:directional_consensus` | `3.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `sentiment:largest_move_share` | `3.4%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `sentiment:largest_move_tie_count` | `5.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `sentiment:magnitude_mad` | `5.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `sentiment:return_interquartile_range` | `5.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |
| `sentiment:return_mad` | `5.0%` | `100.0%` | `[0.000, 0.000]` | `DEAD` |

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `59313`
- Direct relationships: `27284`; inverse relationships: `32029`
- Signed real mean: `0.010`; signed shuffled mean: `-0.000`
- 95th percentile of `|null r|`: `0.096`
- Real `|r|` above that empirical bound: `9.4%`
- KS distance in `|r|` space: `0.080`

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

- Held-out emissions: `1561` across `11` regions
- Excitation strength: mean `48168341956291.125`, peak `37220760605381776.000`, runner-up margin `23802700783040.422`
- Active cell coverage: `100.0%` mean
- Maximum observed token share: `34.0%`
- Real transition entropy: `2.568` bits
- Empirical dwell-block null mean: `2.731` bits
- Difference (null - real): `0.163` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

#### 1. Statistical Separation
- Detections: `1071` (chop, down, flat, up, up_friction)
- A->B Ignition: `MEASURED`, event/control `109/2723`, JSD `0.027` vs null95 `0.039`
- B->C Exhaustion: `MEASURED`, event/control `138/88`, JSD `0.143` vs null95 `0.058`
- Supplemental non-excursion background observations: `974`

#### 2. Predictive Skill (Anticipation)
- Balanced Accuracy: `55.1%` | MCC: `0.039`
- Precision / Recall: `4.7%` / `53.2%`
- Mutual Information (Predictive Gain): `0.003` bits
- Prior Base Rate: `3.8%` | Top Precursor Tokens: `R09, R04, R06, R00, R01`

#### 3. Economic Relevance (Friction Clearance)
- Evaluated Excursions: `94` | Round-Trip Taker Fee: `52.00` bps
- Friction Clearance Rate: `18.1%` (`17` profitable / `77` unprofitable)
- Gross Mean Return: `0.34%` | Net Mean Return after Fees: `-0.18%`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `1071` forming `867` sequential phases (enter: `10`, exit: `16`, wait: `841`)
- Balanced accuracy: `30.4%` vs best baseline (`always_abstain`): `33.3%` (Raw hit rate: `664/867` `76.6%`)
- Matthews Correlation Coefficient (MCC): `0.000`
- Enter action precision / recall: `0.0%` / `0.0%`
- Label-shuffled empirical null balanced accuracy: mean `33.0%`, 95th percentile `38.7%` (empirical p-value: `0.120`)
- Separates from null: `false`
- Post-teach memory retention: `594/867` (`68.5%`)
- Trie topology: `954` nodes, max depth `42`, mean depth `8.8`, branching factor `1.21`
- Basin geometry: records `953`, span `42`, active enter basins `4`, active exit basins `14` (total: `18`)
- Decisiveness: abstention rate `79.0%`, mean confidence `0.009`, mean contrast `17.319`
- Unseen background false-alarm rate: `13.57%` spurious triggers on continuous tape

> ⚠️ **LEARNING DEFICIT:** Prequential balanced accuracy (30.4%) trails baseline policy (33.3%). Memory retention is at 68.5%.

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

### Validation 1: Production-vs-Audit Equivalence

- Replayed ticks: `3665`
- Tokens verified: `3981`
- Token mismatches: `0`
- Metric/brightness mismatches: `0`
- Executable path: `/var/folders/30/bdbpd0hj5wddgkxyx8pbc0mc0000gn/T/go-build1093100569/b001/exe/main`
- Equivalence passed: `true`

### Validation 2: Metric Truthfulness

- Measurements audited: `37516`
- Physical invariant violations: `0`
- Zero-filled midpoints/microprices: `0`
- Synthetic constant time-steps: `0`
- Truthfulness passed: `true`

### Validation 3: Causality & State Isolation

- Future perturbation ticks: `1833`
- Lookahead leakage detected: `true` (first divergence tick: `11`, contaminated: `10032`)
- Cross-symbol contamination: `true`
- Epoch isolation passed: `true`
- Causality passed: `false`

### Validation 4: Grid Dependence & Sensitivity

- Families evaluated (LOFO): `10`
- Duplication resistant: `true`
- Shuffling noise resilient: `true`
- Sensitivity passed: `true`

  - Family `morphology`: Removed JSD = `0.000` bits (dominant: `false`)
  - Family `cvd`: Removed JSD = `0.083` bits (dominant: `false`)
  - Family `sentiment`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `correlation`: Removed JSD = `0.022` bits (dominant: `false`)
  - Family `hawkes`: Removed JSD = `0.088` bits (dominant: `false`)
  - Family `pumpdump`: Removed JSD = `0.006` bits (dominant: `false`)
  - Family `leadlag`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `toxicity`: Removed JSD = `0.003` bits (dominant: `false`)
  - Family `liquidity`: Removed JSD = `0.001` bits (dominant: `false`)
  - Family `depthflow`: Removed JSD = `0.001` bits (dominant: `false`)

