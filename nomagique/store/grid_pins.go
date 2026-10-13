package store

/*
regionMetricPins defines the explicit mapping between individual signal metrics
and their confluence regions (0..11).

The 12 regions detect structural regime transitions for precursor detection:
  - R00: Supercritical Buy Arrival Cascade (Point-Process Ignition / The Spark — Entry Precursor)
  - R01: Pacing Expansion & Criticality Transition (Clock Acceleration / Volume Velocity)
  - R02: Supercritical Sell Arrival Cascade & Panic Cascade (Downward Liquidation — Urgent Exit Precursor)
  - R03: Unilateral Taker Buy Inelasticity (Aggressive Offer Sweep — Breakout Confirmation)
  - R04: Cumulative Volume Delta & Distribution Churn (Taker Absorption / Distribution Trap Precursor)
  - R05: Unilateral Taker Sell Inelasticity & Support Evaporation (Bid Dumping / Floor Collapse — Exit Precursor)
  - R06: Ask Resistance Thinning & Retreat (Offer Vacuum / Ceiling Evaporation — Immediate Breakout Precursor)
  - R07: Frictional Dislocation, Spread Expansion & Geometric Book Deformation (Algorithmic Spoof Grid / Climax Blowout)
  - R08: Bid Support Replenishment & Floor Loading (Limit Accumulation Base — Support Confirmation)
  - R09: Hero Leadership, Advance Concordance & Cohort Siphon (The 24h Riser Engine)
  - R10: Quiescent Compression, Macro Baseline & Background Energy (The Coil — Compression Baseline)
  - R11: Helper Lag, Sympathetic Drift & Systemic Dispersion (Cohort Drag & Market-Wide Breakdown — Risk Precursor)
*/
var regionMetricPins = map[string]map[string]uint8{
	"correlation": {
		"cohort_covariance_score":   11,
		"historical_path_distance":   10,
		"historical_path_percentile": 10,
	},
	"cvd": {
		"midpoint_log_return":          3,
		"flow_aligned_midpoint_return": 4,
		"gross_notional":               4,
		"signed_net_fraction":          4,
	},
	"depthflow": {
		"mutation_activity_imbalance": 7,
		"book_imbalance":              7,
		"net_displayed_flow:bid":      8,
		"touch_notional:bid":          8,
	},
	"hawkes": {
		"arrival_rate:buy":               0,
		"branching_spectral_radius":      0,
		"conditional_intensity_velocity": 1,
		"arrival_rate:sell":              2,
	},
	"leadlag": {
		"led_peer_share":               9,
		"covariance_score_gain_median": 9,
		"defined_peer_count":            9,
		"best_lag_seconds_median":      11,
		"best_lag_seconds_mad":         11,
	},
	"liquidity": {
		"best_ask_price":  6,
		"best_bid_price":  8,
		"relative_spread": 7,
	},
	"morphology": {
		"concentration:ask":   6,
		"entropy:ask":         6,
		"book_shape_distance": 7,
		"book_shape_ks":       7,
		"morphology_change":   7,
		"concentration:bid":   8,
		"entropy:bid":         8,
	},
	"pumpdump": {
		"volume_rate":              1,
		"trade_rate":               1,
		"positive_midpoint_return": 3,
		"negative_midpoint_return": 5,
		"spread_ratio":             10,
	},
	"sentiment": {
		"same_direction_peer_count":     9,
		"breadth":                       9,
		"breadth_velocity":              9,
		"historical_path_distance":      10,
		"historical_path_percentile":    10,
		"opposite_direction_peer_count": 11,
	},
	"toxicity": {
		"retreat_fraction:bid":   5,
		"net_withdrawal_rate:bid": 5,
	},
}
