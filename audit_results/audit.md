# Audit of epoch 1792249581528452000 (workspace/tick/aggregate/1792249581528452000/)

No policy epoch is trained on these ticks.

| stage | result |
|---|---|
| timing | FAIL |
| contract | pass |
| vitality | FAIL |
| redundancy | FAIL |
| sympathy | pass |
| stability | pass |
| dynamics | pass |
| precursor | FAIL |
| cognitive | FAIL |
| truthfulness | pass |
| realization | FAIL |

## audit:timing

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** AVA/USD,
  **DESCRIPTION:** Delay on AVA/USD is unsteady: 7.4% of 27 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** BABY/USD,
  **DESCRIPTION:** Delay on BABY/USD is unsteady: 5.1% of 39 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** CT/USD,
  **DESCRIPTION:** Delay on CT/USD is unsteady: 5.4% of 56 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** CTR/USD,
  **DESCRIPTION:** Delay on CTR/USD is unsteady: 6.9% of 29 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** EIGEN/USD,
  **DESCRIPTION:** Delay on EIGEN/USD is unsteady: 5.9% of 34 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** FET/USD,
  **DESCRIPTION:** Delay on FET/USD is unsteady: 7.7% of 26 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** RSC/USD,
  **DESCRIPTION:** Delay on RSC/USD is unsteady: 8.0% of 25 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** SHIB/USD,
  **DESCRIPTION:** Delay on SHIB/USD is unsteady: 5.1% of 39 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** SNEK/USD,
  **DESCRIPTION:** Delay on SNEK/USD is unsteady: 10.0% of 40 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** SPX/USD,
  **DESCRIPTION:** Delay on SPX/USD is unsteady: 5.8% of 52 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** TEL/USD,
  **DESCRIPTION:** Delay on TEL/USD is unsteady: 7.7% of 26 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** UAI/USD,
  **DESCRIPTION:** Delay on UAI/USD is unsteady: 14.3% of 21 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** WAL/USD,
  **DESCRIPTION:** Delay on WAL/USD is unsteady: 9.8% of 41 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** ZBCN/USD,
  **DESCRIPTION:** Delay on ZBCN/USD is unsteady: 5.6% of 36 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,


## audit:contract


## audit:vitality

- [VIOLATION] vitality.unmeasured,
  **SYMBOL:** sentiment.historical_path_distance,
  **DESCRIPTION:** sentiment.historical_path_distance was 0 on every sample from the active symbols (COPM/USD, Q/USD, IDEX/USD, ALEO/USD, BABYSHARK/USD), so it never measured anything there,
  **METRIC:** ,

- [VIOLATION] vitality.unmeasured,
  **SYMBOL:** sentiment.historical_path_percentile,
  **DESCRIPTION:** sentiment.historical_path_percentile was 0 on every sample from the active symbols (COPM/USD, Q/USD, IDEX/USD, ALEO/USD, BABYSHARK/USD), so it never measured anything there,
  **METRIC:** ,


## audit:redundancy

- [VIOLATION] redundancy.collinear_pair,
  **SYMBOL:** concentration:ask &lt;-&gt; entropy:ask,
  **DESCRIPTION:** Metric pair [concentration:ask &lt;-&gt; entropy:ask] are collinear clones (|r| = -0.9593 &gt;= 0.95 across 13216 observations),
  **METRIC:** ,

- [VIOLATION] redundancy.collinear_pair,
  **SYMBOL:** concentration:bid &lt;-&gt; entropy:bid,
  **DESCRIPTION:** Metric pair [concentration:bid &lt;-&gt; entropy:bid] are collinear clones (|r| = -0.9636 &gt;= 0.95 across 13216 observations),
  **METRIC:** ,


## audit:sympathy


## audit:stability


## audit:dynamics


## audit:precursor

- [VIOLATION] precursor.insufficient_evidence,
  **SYMBOL:** precursor,
  **DESCRIPTION:** 0 precursor and 12487 background tokens over 0 episodes; need 10 of each,
  **METRIC:** ,

- [VIOLATION] excursion.insufficient_evidence,
  **SYMBOL:** excursion,
  **DESCRIPTION:** no up excursions among 0 detected episodes: run detect on the epoch,
  **METRIC:** ,


## audit:cognitive

- [VIOLATION] cognitive.insufficient_evidence,
  **SYMBOL:** cognitive,
  **DESCRIPTION:** no published policies for the epoch: run train on it,
  **METRIC:** ,


## audit:truthfulness


## audit:realization

paper records 0 (0 under other policy epochs), traded paths 0, correlation 0.0000 (p 0.0000), sign agreement 0.000, mean realized - predicted 0.000000

- [VIOLATION] realization.insufficient_evidence,
  **SYMBOL:** realization,
  **DESCRIPTION:** no paper data: no paper/ records were found (run make run with a model, then make paper),
  **METRIC:** ,


## Visualizations & Excursion Dynamics

![Excursion Dynamic Transitions Animation](plots/excursion_transitions.gif)

![Excursion Dynamic Transitions Summary](plots/excursion_transitions_summary.png)

![Audit Summary Scorecard](plots/audit_summary_scorecard.png)

