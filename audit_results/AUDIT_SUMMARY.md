# SYMM Pipeline Empirical Audit

**State:** INCOMPLETE EVIDENCE | **Epoch:** `1791386907328978000` | **Symbol:** `ALL` | **Ticks:** `1000` | **Generated:** `2026-10-08T18:14:47Z`

This report follows [the empirical audit contract](../hindsight/audit/AUDIT_CONTRACT.md): hard mathematical contracts may fail; descriptive stages report measurements; missing evidence is explicit.

| Stage | Question | Experiment state | Observation |
| :--- | :--- | :---: | :--- |
| **0. Contracts** | Do declared hard domains hold? | **VALID** | 0/19273 series breached (0 observations) |
| **1. Vitality** | What raw/canonical evidence actually exists? | **MEASURED** | 19273 raw series; 373 canonical cells; 60 constant canonical cells |
| **2. Sympathy** | Do observed deformations relate beyond a mask-preserving shuffled null? | **MEASURED** | 48482 pairs; |null| p95 0.081; 43.2% real |r| above it; KS 0.397 |
| **3. Grid reproducibility** | Do disjoint periods recover the same co-memberships? | **MEASURED** | ARI 0.073; 100.0% universe overlap (373 shared cells) |
| **4. Token dynamics** | What does a frozen grid emit on unseen tape? | **MEASURED** | 330 emissions; 20 regions; H=2.934 vs null mean 3.084 |
| **5. Precursors** | Are A->B and B->C populations measurable? | **A->B INSUFFICIENT_DATA / B->C INSUFFICIENT_DATA** | A->B N=0/0, JSD 0.000 vs null95 0.000; B->C N=0/0 |
| **6. Cognitive Trie** | Does prequential recall beat baselines and retain memory? | **INSUFFICIENT_DATA** | 0 phases; hits 0/0 (0.0%) vs baseline 0/0 (0.0%); retention 0.0% |

---

### Stage 0: Declared mathematical contracts

- Series checked: `19273`
- Series with hard-domain breaches: `0`
- Breach observations: `0`

> The audit reports the disagreement only. It does not infer a root cause or clamp the observation to fit the contract.

![Stage 0](plots/stage0_metric_contracts.png)

### Stage 1: Observed metric population

- Raw named series: `19273` (varying `17266`, constant `2007`)
- Canonical grid cells: `373` (varying `313`, constant `60`)
- High-correlation canonical pairs shown by the current reference filter: `494`

> Coverage is reported per metric but is not itself a health threshold. High pairwise correlation is not treated as proof that a metric can be removed.

![Stage 1 Vitality](plots/stage1_metric_vitality.png)

![Stage 1 Pair Correlation](plots/stage1_metric_redundancy.png)

### Stage 2: Sympathy against an empirical null

- Simultaneously observed pairs: `48482`
- Direct relationships: `25155`; inverse relationships: `23310`
- Signed real mean: `0.015`; signed shuffled mean: `0.000`
- 95th percentile of `|null r|`: `0.081`
- Real `|r|` above that empirical bound: `43.2%`
- KS distance in `|r|` space: `0.397`

> Missing deformations remain missing in both real and shuffled populations; the null preserves each channel's observation mask.

![Stage 2 Sympathy](plots/stage2_sympathy_null.png)

![Stage 2 Orientation](plots/stage2_orientation_balance.png)

### Stage 3: Grid reproducibility

- Early grid: `373` cells / `20` regions
- Late grid: `373` cells / `20` regions
- Shared universe: `373` cells (`100.0%`)
- Adjusted Rand Index: `0.073`

> Balanced region sizes are enforced by the partitioner and are not presented as empirical evidence. No ARI health cutoff is applied.

![Stage 3](plots/stage3_region_partitioning.png)

### Stage 4: Held-out token dynamics & excitation strength

- Held-out emissions: `330` across `20` regions
- Excitation strength: mean `268.470`, peak `2962.481`, runner-up margin `258.591`
- Active cell coverage: `64.1%` mean
- Maximum observed token share: `19.1%`
- Real transition entropy: `2.934` bits
- Empirical dwell-block null mean: `3.084` bits
- Difference (null - real): `0.150` bits

> The same causal Stream continues across the train/holdout boundary. The report does not turn an entropy difference into a PASS/FAIL cutoff.

![Stage 4 Tokens](plots/stage4_token_dynamics.png)

![Stage 4 Entropy](plots/stage4_transition_entropy.png)

![Stage 4 Matrix](plots/stage4_transition_matrix.png)

### Stage 5: Event-centred precursor populations

- Detections: `2413` (chop, down, flat, up, up_friction)
- A->B: `INSUFFICIENT_DATA`, event/control `0/0`, JSD `0.000`, null95 `0.000`
- B->C: `INSUFFICIENT_DATA`, event/control `0/0`, JSD `0.000`, null95 `0.000`
- Supplemental non-excursion background observations: `0`

> Current trading semantics are long-only: only profitable `up` excursions populate the positive A->B set. Event windows are loaded directly from the archive rather than requiring them to occur inside the first-N audit sample.

![Stage 5](plots/stage5_precursor_separation.png)

### Stage 6: Cognitive Engine & Radix Trie Learning Dynamics

- Evaluated excursions: `0` forming `0` sequential phases (enter: `0`, exit: `0`, wait: `0`)
- Prequential accuracy: `0/0` (`0.0%`) vs best constant policy (``): `0/0` (`0.0%`)
- Label-shuffled empirical null: mean `0.0` hits, std `0.0`, 95th percentile `0.0` hits (empirical p-value: `0.000`)
- Separates from null: `false`
- Post-teach memory retention: `0/0` (`0.0%`)
- Trie topology: `0` nodes, max depth `0`, mean depth `0.0`, branching factor `0.00`
- Basin geometry: records `0`, span `0`, active enter basins `0`, active exit basins `0` (total: `0`)
- Decisiveness: abstention rate `0.0%`, mean confidence `0.000`, mean contrast `0.000`
- Unseen background false-alarm rate: `0.00%` spurious triggers on continuous tape

> Prequential recall evaluates the trie strictly before learning each phase. Abstention is the appropriate stance on controls, not a terminal action. Shuffled null tests whether sequential prefix structure holds predictive edge over class priors.

![Stage 6 Skill](plots/stage6_trie_skill.png)

![Stage 6 Structure](plots/stage6_trie_structure.png)

