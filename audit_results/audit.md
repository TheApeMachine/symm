# Audit of epoch 1792252538780196000 (workspace/tick/aggregate/1792252538780196000/)

44 sequences from policy/current.

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
| cognitive | pass |
| truthfulness | pass |
| realization | FAIL |

## audit:timing

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** ACU/USD,
  **DESCRIPTION:** Delay on ACU/USD is unsteady: 6.1% of 33 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** AKE/USD,
  **DESCRIPTION:** Delay on AKE/USD is unsteady: 7.1% of 28 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** BABY/USD,
  **DESCRIPTION:** Delay on BABY/USD is unsteady: 6.7% of 30 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** CT/USD,
  **DESCRIPTION:** Delay on CT/USD is unsteady: 9.1% of 33 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** ENA/USD,
  **DESCRIPTION:** Delay on ENA/USD is unsteady: 14.3% of 21 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** MEGA/USD,
  **DESCRIPTION:** Delay on MEGA/USD is unsteady: 14.3% of 21 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** POL/USD,
  **DESCRIPTION:** Delay on POL/USD is unsteady: 5.4% of 37 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** PYTH/USD,
  **DESCRIPTION:** Delay on PYTH/USD is unsteady: 6.9% of 29 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** SENT/USD,
  **DESCRIPTION:** Delay on SENT/USD is unsteady: 6.5% of 31 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** SPX/USD,
  **DESCRIPTION:** Delay on SPX/USD is unsteady: 10.7% of 28 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** WLD/USD,
  **DESCRIPTION:** Delay on WLD/USD is unsteady: 8.0% of 25 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** XDC/USD,
  **DESCRIPTION:** Delay on XDC/USD is unsteady: 5.6% of 36 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** XMR/USD,
  **DESCRIPTION:** Delay on XMR/USD is unsteady: 11.8% of 34 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,

- [VIOLATION] timing.latency_excess,
  **SYMBOL:** ZRO/USD,
  **DESCRIPTION:** Delay on ZRO/USD is unsteady: 7.1% of 28 packets were far from its usual delay, above the 5% allowance,
  **METRIC:** ,


## audit:contract


## audit:vitality

- [VIOLATION] vitality.unmeasured,
  **SYMBOL:** sentiment.historical_path_distance,
  **DESCRIPTION:** sentiment.historical_path_distance was 0 on every sample from the active symbols (BDXN/USD, MEZO/USD, ICNT/USD, ALTHEA/USD, Q/USD), so it never measured anything there,
  **METRIC:** ,

- [VIOLATION] vitality.unmeasured,
  **SYMBOL:** sentiment.historical_path_percentile,
  **DESCRIPTION:** sentiment.historical_path_percentile was 0 on every sample from the active symbols (BDXN/USD, MEZO/USD, ICNT/USD, ALTHEA/USD, Q/USD), so it never measured anything there,
  **METRIC:** ,


## audit:redundancy

- [VIOLATION] redundancy.collinear_pair,
  **SYMBOL:** concentration:ask &lt;-&gt; entropy:ask,
  **DESCRIPTION:** Metric pair [concentration:ask &lt;-&gt; entropy:ask] are collinear clones (|r| = -0.9568 &gt;= 0.95 across 9801 observations),
  **METRIC:** ,

- [VIOLATION] redundancy.collinear_pair,
  **SYMBOL:** concentration:bid &lt;-&gt; entropy:bid,
  **DESCRIPTION:** Metric pair [concentration:bid &lt;-&gt; entropy:bid] are collinear clones (|r| = -0.9607 &gt;= 0.95 across 9801 observations),
  **METRIC:** ,


## audit:sympathy


## audit:stability


## audit:dynamics


## audit:precursor

- [VIOLATION] precursor.insufficient_evidence,
  **SYMBOL:** precursor,
  **DESCRIPTION:** 0 precursor and 9135 background tokens over 0 episodes; need 10 of each,
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

