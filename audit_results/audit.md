# Audit of epoch 1792187257188073000 (workspace/tick/aggregate/1792187257188073000/)

| stage | result |
|---|---|
| timing | FAIL |
| contract | pass |
| vitality | FAIL |
| redundancy | FAIL |
| sympathy | FAIL |
| stability | FAIL |
| dynamics | FAIL |
| precursor | FAIL |
| cognitive | FAIL |
| truthfulness | pass |

## audit:timing

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** ZORA/USD,
  **DESCRIPTION:** Latency spike: drift 149.83 ms deviates by 83.97 sigma from mean 49.46 ms (bound 4.47),
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** STRK/USD,
  **DESCRIPTION:** Latency spike: drift 107.64 ms deviates by 7.61 sigma from mean 97.76 ms (bound 4.47),
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** WAL/USD,
  **DESCRIPTION:** Latency spike: drift 155.41 ms deviates by 42.37 sigma from mean 55.69 ms (bound 4.47),
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** STRK/USD,
  **DESCRIPTION:** Latency spike: drift 57.41 ms deviates by 10.73 sigma from mean 101.03 ms (bound 4.47),
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** SOL/USD,
  **DESCRIPTION:** Latency spike: drift 193.62 ms deviates by 5.13 sigma from mean 67.89 ms (bound 4.47),
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** TIA/USD,
  **DESCRIPTION:** Latency spike: drift 114.05 ms deviates by 23.56 sigma from mean 52.15 ms (bound 4.47),
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** IDEX/USD,
  **DESCRIPTION:** Latency spike: drift 108.91 ms deviates by 60.64 ms from low-dispersion mean 48.26 ms,
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** PUMP/USD,
  **DESCRIPTION:** Latency spike: drift 91.81 ms deviates by 14.56 sigma from mean 52.73 ms (bound 4.47),
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** DOT/USD,
  **DESCRIPTION:** Latency spike: drift 91.95 ms deviates by 14.58 sigma from mean 52.19 ms (bound 4.47),
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** NEAR/USD,
  **DESCRIPTION:** Latency spike: drift 156.46 ms deviates by 75.83 sigma from mean 58.32 ms (bound 4.47),
  **METRIC:** ,

- [VIOLATION] timing.latency_spike,
  **SYMBOL:** DOT/USD,
  **DESCRIPTION:** Latency spike: drift 163.43 ms deviates by 10.25 sigma from mean 54.68 ms (bound 4.47),
  **METRIC:** ,


## audit:contract


## audit:vitality

- [VIOLATION] vitality.dead_metric,
  **SYMBOL:** price|ZORA/USD|best_price:bid,
  **DESCRIPTION:** Metric price|ZORA/USD|best_price:bid is dead/constant (variance = 0.0 across 56 observations, range [0.007, 0.007]),
  **METRIC:** ,


## audit:redundancy

- [VIOLATION] redundancy.insufficient_evidence,
  **SYMBOL:** redundancy,
  **DESCRIPTION:** no metric pairs observed,
  **METRIC:** ,


## audit:sympathy

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair BTC/USD:price:best_price:ask&lt;-&gt;ETH/USD:price:best_price:ask fails sympathy test: concordance = 0.2407 &lt;= 0 or p-value = 0.0588 &gt; 0.05 vs shuffled null (z-score = 1.68, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair BTC/USD:price:best_price:ask&lt;-&gt;SOL/USD:price:best_price:ask fails sympathy test: concordance = 0.1623 &lt;= 0 or p-value = 0.1373 &gt; 0.05 vs shuffled null (z-score = 1.21, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair BTC/USD:price:best_price:ask&lt;-&gt;STRK/USD:price:best_price:ask fails sympathy test: concordance = 0.1952 &lt;= 0 or p-value = 0.4118 &gt; 0.05 vs shuffled null (z-score = 1.39, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair BTC/USD:price:best_price:ask&lt;-&gt;ZORA/USD:price:best_price:ask fails sympathy test: concordance = 0.1589 &lt;= 0 or p-value = 0.2157 &gt; 0.05 vs shuffled null (z-score = 1.39, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair BTC/USD:price:best_price:bid&lt;-&gt;DOT/USD:price:best_price:bid fails sympathy test: concordance = 0.1581 &lt;= 0 or p-value = 0.4118 &gt; 0.05 vs shuffled null (z-score = 1.00, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair BTC/USD:price:best_price:bid&lt;-&gt;ETH/USD:price:best_price:bid fails sympathy test: concordance = 0.2407 &lt;= 0 or p-value = 0.0980 &gt; 0.05 vs shuffled null (z-score = 1.68, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair BTC/USD:price:best_price:bid&lt;-&gt;SOL/USD:price:best_price:bid fails sympathy test: concordance = 0.1863 &lt;= 0 or p-value = 0.1961 &gt; 0.05 vs shuffled null (z-score = 1.39, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair BTC/USD:price:best_price:bid&lt;-&gt;STRK/USD:price:best_price:bid fails sympathy test: concordance = 0.1952 &lt;= 0 or p-value = 0.4706 &gt; 0.05 vs shuffled null (z-score = 1.39, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:ask&lt;-&gt;ETH/USD:price:best_price:ask fails sympathy test: concordance = -0.0098 &lt;= 0 or p-value = 0.8235 &gt; 0.05 vs shuffled null (z-score = -0.06, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:ask&lt;-&gt;SOL/USD:price:best_price:ask fails sympathy test: concordance = 0.0730 &lt;= 0 or p-value = 0.4314 &gt; 0.05 vs shuffled null (z-score = 0.48, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:ask&lt;-&gt;STRK/USD:price:best_price:ask fails sympathy test: concordance = -0.0096 &lt;= 0 or p-value = 0.9216 &gt; 0.05 vs shuffled null (z-score = -0.05, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:ask&lt;-&gt;WAL/USD:price:best_price:ask fails sympathy test: concordance = 0.0385 &lt;= 0 or p-value = 0.5882 &gt; 0.05 vs shuffled null (z-score = 0.18, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:ask&lt;-&gt;XRP/USD:price:best_price:ask fails sympathy test: concordance = -0.0037 &lt;= 0 or p-value = 0.8431 &gt; 0.05 vs shuffled null (z-score = -0.02, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:ask&lt;-&gt;ZORA/USD:price:best_price:ask fails sympathy test: concordance = 0.0633 &lt;= 0 or p-value = 0.6078 &gt; 0.05 vs shuffled null (z-score = 0.44, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:bid&lt;-&gt;ETH/USD:price:best_price:bid fails sympathy test: concordance = 0.1136 &lt;= 0 or p-value = 0.5686 &gt; 0.05 vs shuffled null (z-score = 0.71, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:bid&lt;-&gt;SOL/USD:price:best_price:bid fails sympathy test: concordance = 0.0965 &lt;= 0 or p-value = 0.4706 &gt; 0.05 vs shuffled null (z-score = 0.64, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:bid&lt;-&gt;STRK/USD:price:best_price:bid fails sympathy test: concordance = 0.1952 &lt;= 0 or p-value = 0.1569 &gt; 0.05 vs shuffled null (z-score = 1.12, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:bid&lt;-&gt;WAL/USD:price:best_price:bid fails sympathy test: concordance = 0.2093 &lt;= 0 or p-value = 0.3137 &gt; 0.05 vs shuffled null (z-score = 1.00, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair DOT/USD:price:best_price:bid&lt;-&gt;XRP/USD:price:best_price:bid fails sympathy test: concordance = -0.0854 &lt;= 0 or p-value = 0.8431 &gt; 0.05 vs shuffled null (z-score = -0.44, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair ETH/USD:price:best_price:ask&lt;-&gt;SOL/USD:price:best_price:ask fails sympathy test: concordance = 0.1897 &lt;= 0 or p-value = 0.1961 &gt; 0.05 vs shuffled null (z-score = 1.35, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair ETH/USD:price:best_price:ask&lt;-&gt;WAL/USD:price:best_price:ask fails sympathy test: concordance = 0.2557 &lt;= 0 or p-value = 0.1373 &gt; 0.05 vs shuffled null (z-score = 1.47, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair ETH/USD:price:best_price:ask&lt;-&gt;XRP/USD:price:best_price:ask fails sympathy test: concordance = 0.2534 &lt;= 0 or p-value = 0.1961 &gt; 0.05 vs shuffled null (z-score = 1.34, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair ETH/USD:price:best_price:ask&lt;-&gt;ZORA/USD:price:best_price:ask fails sympathy test: concordance = 0.2217 &lt;= 0 or p-value = 0.1176 &gt; 0.05 vs shuffled null (z-score = 1.77, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair ETH/USD:price:best_price:bid&lt;-&gt;SOL/USD:price:best_price:bid fails sympathy test: concordance = 0.2115 &lt;= 0 or p-value = 0.1765 &gt; 0.05 vs shuffled null (z-score = 1.51, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair ETH/USD:price:best_price:bid&lt;-&gt;STRK/USD:price:best_price:bid fails sympathy test: concordance = 0.3218 &lt;= 0 or p-value = 0.1176 &gt; 0.05 vs shuffled null (z-score = 1.93, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair ETH/USD:price:best_price:bid&lt;-&gt;WAL/USD:price:best_price:bid fails sympathy test: concordance = 0.0838 &lt;= 0 or p-value = 0.8431 &gt; 0.05 vs shuffled null (z-score = 0.48, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair ETH/USD:price:best_price:bid&lt;-&gt;XRP/USD:price:best_price:bid fails sympathy test: concordance = 0.2366 &lt;= 0 or p-value = 0.2745 &gt; 0.05 vs shuffled null (z-score = 1.25, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair SOL/USD:price:best_price:ask&lt;-&gt;STRK/USD:price:best_price:ask fails sympathy test: concordance = 0.1299 &lt;= 0 or p-value = 0.7647 &gt; 0.05 vs shuffled null (z-score = 0.85, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair SOL/USD:price:best_price:ask&lt;-&gt;WAL/USD:price:best_price:ask fails sympathy test: concordance = 0.1729 &lt;= 0 or p-value = 0.3922 &gt; 0.05 vs shuffled null (z-score = 1.15, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair SOL/USD:price:best_price:ask&lt;-&gt;XRP/USD:price:best_price:ask fails sympathy test: concordance = 0.1709 &lt;= 0 or p-value = 0.3922 &gt; 0.05 vs shuffled null (z-score = 1.07, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair SOL/USD:price:best_price:ask&lt;-&gt;ZORA/USD:price:best_price:ask fails sympathy test: concordance = 0.1458 &lt;= 0 or p-value = 0.2941 &gt; 0.05 vs shuffled null (z-score = 1.25, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair SOL/USD:price:best_price:bid&lt;-&gt;WAL/USD:price:best_price:bid fails sympathy test: concordance = 0.1257 &lt;= 0 or p-value = 0.3922 &gt; 0.05 vs shuffled null (z-score = 0.83, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair SOL/USD:price:best_price:bid&lt;-&gt;XRP/USD:price:best_price:bid fails sympathy test: concordance = 0.0658 &lt;= 0 or p-value = 0.8627 &gt; 0.05 vs shuffled null (z-score = 0.41, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair STRK/USD:price:best_price:ask&lt;-&gt;WAL/USD:price:best_price:ask fails sympathy test: concordance = -0.2191 &lt;= 0 or p-value = 0.3529 &gt; 0.05 vs shuffled null (z-score = -1.26, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair STRK/USD:price:best_price:ask&lt;-&gt;XRP/USD:price:best_price:ask fails sympathy test: concordance = -0.1014 &lt;= 0 or p-value = 0.6863 &gt; 0.05 vs shuffled null (z-score = -0.54, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair STRK/USD:price:best_price:ask&lt;-&gt;ZORA/USD:price:best_price:ask fails sympathy test: concordance = -0.0154 &lt;= 0 or p-value = 0.8824 &gt; 0.05 vs shuffled null (z-score = -0.13, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair STRK/USD:price:best_price:bid&lt;-&gt;WAL/USD:price:best_price:bid fails sympathy test: concordance = 0.2084 &lt;= 0 or p-value = 0.4510 &gt; 0.05 vs shuffled null (z-score = 1.20, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair STRK/USD:price:best_price:bid&lt;-&gt;XRP/USD:price:best_price:bid fails sympathy test: concordance = 0.2987 &lt;= 0 or p-value = 0.2157 &gt; 0.05 vs shuffled null (z-score = 1.58, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair WAL/USD:price:best_price:ask&lt;-&gt;XRP/USD:price:best_price:ask fails sympathy test: concordance = -0.0261 &lt;= 0 or p-value = 0.8627 &gt; 0.05 vs shuffled null (z-score = -0.15, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair WAL/USD:price:best_price:ask&lt;-&gt;ZORA/USD:price:best_price:ask fails sympathy test: concordance = 0.1270 &lt;= 0 or p-value = 0.3725 &gt; 0.05 vs shuffled null (z-score = 1.00, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair WAL/USD:price:best_price:bid&lt;-&gt;XRP/USD:price:best_price:bid fails sympathy test: concordance = 0.1026 &lt;= 0 or p-value = 0.4510 &gt; 0.05 vs shuffled null (z-score = 0.59, N=50),
  **METRIC:** ,

- [VIOLATION] sympathy.no_concordance,
  **SYMBOL:** cross_asset,
  **DESCRIPTION:** Cross-asset pair XRP/USD:price:best_price:ask&lt;-&gt;ZORA/USD:price:best_price:ask fails sympathy test: concordance = 0.2357 &lt;= 0 or p-value = 0.0784 &gt; 0.05 vs shuffled null (z-score = 1.78, N=50),
  **METRIC:** ,


## audit:stability

- [VIOLATION] grid.insufficient_evidence,
  **SYMBOL:** grid,
  **DESCRIPTION:** 466 scorable frames (0 warming up) and 0 activations; need 20 of each,
  **METRIC:** ,


## audit:dynamics

- [VIOLATION] dynamics.insufficient_evidence,
  **SYMBOL:** dynamics,
  **DESCRIPTION:** 0 tokens; need 20,
  **METRIC:** ,


## audit:precursor

- [VIOLATION] precursor.insufficient_evidence,
  **SYMBOL:** precursor,
  **DESCRIPTION:** 0 precursor and 0 background tokens over 0 episodes; need 10 of each,
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


## Visualizations & Excursion Dynamics

![Excursion Dynamic Transitions Animation](plots/excursion_transitions.gif)

![Excursion Dynamic Transitions Summary](plots/excursion_transitions_summary.png)

![Audit Summary Scorecard](plots/audit_summary_scorecard.png)

