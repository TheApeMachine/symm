# signals

## Enter

### Detector

- [x] pumpdump.volume_rate
- [x] pumpdump.volume_ratio
- [x] pumpdump.trade_rate
- [x] hawkes.arrival_rate:buy
- [x] pumpdump.positive_midpoint_return
- [x] sentiment.same_direction_peer_count

Volume & trade arrival velocity burst.
Volume ratio expansion ($> 1.0$, multiples of baseline volume).
Sudden tick in buyer arrival intensity.
First micro-nudge above consolidation.
Initial sign of cohort peers awakening.

### Support

- [x] cvd.signed_net_fraction
- [x] depthflow.net_displayed_flow:bid
- [x] depthflow.touch_imbalance
- [x] hawkes.branching_spectral_radius
- [x] sentiment.same_direction_peer_count
- [x] leadlag.led_peer_share

Aggressor dominance ($> 0$, heavy market buy pressure).
Limit bids stepping up behind the price to support the move.
Touch asymmetry ($> 0$, resting bids at touch outnumbering asks).
Supercritical feedback loop ($\rho \ge 1.0$: trades triggering more trades).
Cohort peers confirming the move in lockstep.
Hero Leadership: leadlag.led_peer_share $> 0.5$ (focal asset is the vanguard leader of the cohort, moving before its peers).

### Opposition

- [x] cvd.gross_notional
- [x] cvd.midpoint_log_return
- [x] toxicity.retreat_fraction:bid
- [x] depthflow.book_imbalance
- [x] depthflow.touch_notional:ask
- [x] sentiment.opposite_direction_peer_count
- [x] sentiment.breadth_velocity
- [x] morphology.concentration:ask
- [x] morphology.book_shape_distance
- [x] morphology.entropy:bid
- [x] leadlag.best_lag_seconds_median

The Iceberg Trap: gross_notional is massive, but midpoint_log_return is flat (buying into a hidden sell wall).
The Spoof Trap: retreat_fraction:bid ticks up (bid floor flees upon touch).
The Ask Wall: book_imbalance < 0 (colossal ask stack 1% above touch; no runway) or morphology.concentration:ask approaches 1.0 (monolithic sell wall capping upward movement).
The Hollow Touch Trap: touch_notional:ask is negligible (buying market orders sweeps thin levels, causing violent slippage).
The Shape Overhang: morphology.book_shape_distance is wide while morphology.entropy:bid is low (brittle, hollow bid cushion facing an asymmetric overhead ask overhang).
Contagion Drag: opposite_direction_peer_count dominates and breadth_velocity < 0 (buying while the broader cohort is draining).
The Helper Trap: leadlag.best_lag_seconds_median $> 0$ (focal asset is trailing behind peers; buying a follower carries high risk of buying the top of a peer-driven impulse).

## Exit

### Detector

- [x] pumpdump.negative_midpoint_return
- [x] hawkes.conditional_intensity_velocity

First sign of a rejection wick or stall at local high.
Arrival acceleration flips negative ($< 0$: trades spacing out).

### Support

- [x] toxicity.net_withdrawal_rate:bid
- [x] liquidity.relative_spread
- [x] pumpdump.spread_ratio
- [x] cvd.flow_aligned_midpoint_return
- [x] hawkes.arrival_rate:sell
- [x] morphology.concentration:ask
- [x] morphology.entropy:bid
- [x] morphology.morphology_change
- [x] leadlag.best_lag_seconds_median

Bid cancellation velocity vertical (the floor beneath us is vanishing).
Spread blowout (market makers stepping back).
Peak distribution: heavy buy volume printing, but price drifting down ($< 0$).
Aggressive seller arrival velocity accelerating.
Geometric Evaporation: morphology.morphology_change spikes as morphology.entropy:bid collapses and morphology.concentration:ask thickens (the book shape is actively warping and closing down overhead).
Leadership Decay: leadlag.best_lag_seconds_median slips from negative into positive lag (the focal asset loses its vanguard status and begins trailing the cohort, signaling exhaustion).

### Opposition

- [x] depthflow.net_displayed_flow:bid
- [x] cvd.signed_net_fraction
- [x] toxicity.retreat_fraction:bid
- [x] sentiment.same_direction_peer_count
- [x] morphology.entropy:bid
- [x] morphology.concentration:bid
- [x] leadlag.led_peer_share

Absorption reload: bids reloading and absorbing market sells at higher level ($> 0$).
Trickle flow: sells are weak trickle orders, not aggressive market dumps.
Steadfast makers: retreat_fraction:bid is low (buyers standing their ground).
Cohort peers are holding strong.
Deep Cushion: high morphology.entropy:bid and robust morphology.concentration:bid (a broad, stable bed of resting bids is actively buffering the sell pressure).
Hero Resilience: leadlag.led_peer_share remains dominant ($> 0.5$, $\tau < 0$), showing the focal asset is still the active driver of market momentum.

## Regime Oracle

### Coiled Compression

- [x] pumpdump.spread_ratio
- [x] pumpdump.trade_rate
- [x] depthflow.book_imbalance
- [x] morphology.book_shape_distance
- [x] morphology.entropy:bid
- [x] morphology.entropy:ask
- [x] morphology.morphology_change
- [x] leadlag.best_lag_seconds_median
- [x] leadlag.best_lag_seconds_mad

Spread unusually tight ($< 1.0$).
Low baseline trade rate (calm before the storm).
Resting depth balanced between bid and ask ($\approx 0$).
Bilateral geometric symmetry (morphology.book_shape_distance $\approx 0$), high entropy on both sides (dense, uniform, thick depth), and zero structural shape drift (morphology.morphology_change $\approx 0$).
Temporal Lockstep: leadlag.best_lag_seconds_median $\approx 0$ with minimal dispersion (leadlag.best_lag_seconds_mad $\approx 0$), confirming tight cross-asset synchronization with no breakaway leader.

### Explosive Ignition

- [x] hawkes.branching_spectral_radius
- [x] pumpdump.volume_rate
- [x] hawkes.arrival_rate:buy
- [x] hawkes.arrival_rate:sell
- [x] leadlag.led_peer_share
- [x] leadlag.covariance_score_gain_median

Supercritical feedback cascade ($\rho \ge 1.0$).
Explosive volume turnover expansion.
Directional arrival intensity accelerating.
Decisive Breakaway Leadership: leadlag.led_peer_share surges toward 1.0 backed by elevated leadlag.covariance_score_gain_median, confirming that a single vanguard asset has broken out from the pack with strong asymmetric predictive gain.

### Turbulent Friction

- [x] liquidity.relative_spread
- [x] pumpdump.volume_rate
- [x] depthflow.mutation_activity_imbalance
- [x] morphology.morphology_change
- [x] morphology.book_shape_ks

Wide relative spread with high turnover (unpredictable chop).
Erratic book mutations (cancellation and modification activity imbalance).
Erratic book shape warping (morphology.morphology_change $> 0$) and extreme cumulative divergence between bid and ask distributions (morphology.book_shape_ks).

### Dead / Starved

- [x] pumpdump.trade_rate

Trade arrival frequency near zero.
Inter-arrival intervals long, trade hurdle not cleared.

## History Oracle

### Grammar Rule Induction

- [x] strategy.grammar

Sequitur grammar engine: induces words and sentences from atomic event letter sequences.
Maps active structural rule completions ($R_k$) to historical empirical outcome distributions.

### Trajectory Recurrence

- [x] sentiment.historical_path_distance
- [x] sentiment.historical_path_percentile
- [x] correlation.historical_path_distance
- [x] correlation.historical_path_percentile

Minimum Description Length (MDL) path distance to nearest historical multi-signal episode.
Percentile rank: familiar recurring pattern ($< 5\%$) vs high novelty ($> 95\%$).

## Risk Oracle

### Friction Hurdle

- [x] liquidity.relative_spread
- [x] liquidity.fee_rate
- [x] liquidity.friction_hurdle

Relative spread plus round-trip taker fees relative to expected move target.
Total round-trip friction hurdle: relative_spread + 2*fee_rate.
Rejects trades where friction makes expected value negative.

### Macro Contagion

- [x] sentiment.breadth
- [x] sentiment.breadth_velocity
- [x] sentiment.opposite_direction_peer_count
- [x] correlation.cohort_covariance_score
- [x] leadlag.best_lag_seconds_median
- [x] leadlag.best_lag_seconds_mad

Market breadth $< 0.20$ (80% of entire market dumping).
Breadth velocity $< 0$ (market-wide liquidity draining rapidly).
Cohort peer divergence (peers moving opposite to target asset).
Cohort covariance score $> 0.85$ (systemic lockstep; Bitcoin gravity dragging microcaps).
Helper Vulnerability: leadlag.best_lag_seconds_median $> 0$ with elevated leadlag.best_lag_seconds_mad (trailing laggard with disorganized temporal response; highly exposed to downstream macro selloffs).

### Capacity / Exit Liquidity

- [x] depthflow.touch_notional:bid
- [x] depthflow.touch_notional:ask
- [x] morphology.concentration:bid
- [x] morphology.entropy:ask

Available quote notional at best bid compared to intended position size (selling capacity).
Available quote notional at best ask compared to intended entry size (buying capacity).
Fragile Touch Depth: high morphology.concentration:bid warns that resting depth behind touch is hollow once the first level is taken.
Overhead Resistance: thick morphology.entropy:ask confirms heavy resistance bands overhead that will consume exit momentum.
Rejects trades where exit will crash the book or cause excessive slippage.