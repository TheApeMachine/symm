# Audit of epoch 1792256892574441000 (workspace/tick/aggregate/1792256892574441000/)

2 sequences from policy/current.

| stage | result |
|---|---|
| timing | FAIL |
| contract | pass |
| vitality | pass |
| redundancy | FAIL |
| sympathy | pass |
| stability | pass |
| dynamics | FAIL |
| precursor | FAIL |
| cognitive | pass |
| truthfulness | pass |
| realization | FAIL |

## audit:timing

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** A/USD,
  **DESCRIPTION:** Delay on A/USD is unsteady: 7.1% of 28 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** CAP/USD,
  **DESCRIPTION:** Delay on CAP/USD is unsteady: 19.0% of 21 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** INJ/USD,
  **DESCRIPTION:** Delay on INJ/USD is unsteady: 7.3% of 41 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** LINK/USD,
  **DESCRIPTION:** Delay on LINK/USD is unsteady: 6.7% of 45 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** ZAMA/USD,
  **DESCRIPTION:** Delay on ZAMA/USD is unsteady: 7.5% of 40 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,


## audit:contract


## audit:vitality


## audit:redundancy

- [VIOLATION] redundancy.collinear_pair,
  **SYMBOL:** concentration:ask &lt;-&gt; entropy:ask,
  **DESCRIPTION:** Metric pair [concentration:ask &lt;-&gt; entropy:ask] are collinear clones (|r| = -0.9557 &gt;= 0.95 across 5483 observations),
  **METRIC:** ,

- [VIOLATION] redundancy.collinear_pair,
  **SYMBOL:** concentration:bid &lt;-&gt; entropy:bid,
  **DESCRIPTION:** Metric pair [concentration:bid &lt;-&gt; entropy:bid] are collinear clones (|r| = -0.9525 &gt;= 0.95 across 5483 observations),
  **METRIC:** ,


## audit:sympathy


## audit:stability


## audit:dynamics

- [VIOLATION] dynamics.no_structure_vs_null,
  **SYMBOL:** dynamics,
  **DESCRIPTION:** Token order matches a shuffled copy of the same tokens: entropy 2.789 bits, shuffled mean 2.790 bits (p = 0.353),
  **METRIC:** ,


## audit:precursor

- [VIOLATION] precursor.insufficient_evidence,
  **SYMBOL:** precursor,
  **DESCRIPTION:** 0 precursor and 4934 background tokens over 0 episodes; need 10 of each,
  **METRIC:** ,

- [VIOLATION] excursion.insufficient_evidence,
  **SYMBOL:** excursion,
  **DESCRIPTION:** no up excursions among 0 detected episodes: run detect on the epoch,
  **METRIC:** ,


## audit:cognitive


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

