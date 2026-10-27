package store

/*
regionMetricPins defines the explicit mapping between individual signal metrics
and their confluence regions (0..14) matching SIGNALS.md:

ENTER:
  - R00: Detector (The Spark: arrival rates, positive return, volume burst)
  - R01: Support (Vanguard & Flow: net displayed bids, signed CVD, peer vanguard)
  - R02: Opposition (Walls & Spoofs: gross absorption wall, ask overhang, bid retreats)

EXIT:
  - R03: Detector (Rejection & Stall: negative midpoint return, arrival deceleration)
  - R04: Support (Support Evaporation: bid withdrawal rate, sell arrival velocity, shape collapse)
  - R05: Opposition (Absorption Reload: resting bid concentration, bid entropy cushion)

REGIME:
  - R06: Coiled Compression (Quiescent Balance: book balance, uniform depth entropy, tight spread ratio)
  - R07: Explosive Ignition (Supercritical Feedback: branching ratio, covariance score gain)
  - R08: Turbulent Friction (Chop & Warping: book mutation imbalance, shape KS divergence)
  - R09: Dead / Starved (Hold-relative tape fade: trade rate vs entry baseline)

HISTORY:
  - R10: Macro Trajectory Recurrence (Cross-asset covariance MDL path distance & percentile)
  - R11: Sentiment Trajectory Recurrence (Market breadth MDL path distance & percentile)

RISK:
  - R12: Friction Hurdle (Relative spread hurdle, quote levels)
  - R13: Macro Contagion (Market-wide breadth collapse, peer divergence, helper lag)
  - R14: Capacity / Exit Liquidity (Touch quote notional at bid)

A label pinned to no regions ({}) is declared: its signal writes it, the grid
leaves it out of energy and brightness. A label missing here is an error.
*/
var regionMetricPins = map[string]map[string][]uint8{
	"correlation": {
		"cohort_covariance_score":    {13},
		"historical_path_distance":   {10},
		"historical_path_percentile": {10},
	},
	"cvd": {
		"midpoint_log_return":          {0},
		"flow_aligned_midpoint_return": {3},
		"gross_notional":               {2},
		"signed_net_fraction":          {1, 5},
	},
	"depthflow": {
		"mutation_activity_imbalance": {8},
		"book_imbalance":              {1, 6},
		"net_displayed_flow:bid":      {1, 5},
		"touch_notional:bid":          {14},
		"touch_notional:ask":          {2, 14},
		"touch_imbalance":             {1, 5},
	},
	"hawkes": {
		"arrival_rate:buy":               {0, 7},
		"arrival_rate:sell":              {3, 4},
		"branching_spectral_radius":      {1, 7},
		"conditional_intensity_velocity": {0, 3},
		"conditional_intensity":          {},
		"background_rate":                {},
		"excitation_decay":               {},
	},
	"leadlag": {
		"led_peer_share":               {1, 7},
		"covariance_score_gain_median": {7},
		"best_lag_seconds_median":      {6, 13},
		"best_lag_seconds_mad":         {8, 13},
		"defined_peer_count":           {},
	},
	"liquidity": {
		"best_price:bid":  {},
		"best_price:ask":  {},
		"relative_spread": {6, 8, 12},
		"fee_rate":        {12},
		"friction_hurdle": {12},
	},
	"morphology": {
		"concentration:ask":   {2},
		"concentration:bid":   {5, 14},
		"entropy:bid":         {5, 6},
		"entropy:ask":         {6, 14},
		"book_shape_distance": {6, 8},
		"book_shape_ks":       {8},
		"morphology_change":   {4, 8},
	},
	"pumpdump": {
		"volume_rate":              {0, 7},
		"volume_ratio":             {0, 7},
		"trade_rate":               {0, 9},
		"positive_midpoint_return": {0},
		"negative_midpoint_return": {3},
		"spread_ratio":             {6, 8},
	},
	"sentiment": {
		"same_direction_peer_count":     {0, 1},
		"opposite_direction_peer_count": {2, 13},
		"breadth":                       {13},
		"breadth_velocity":              {8, 13},
		"historical_path_distance":      {11},
		"historical_path_percentile":    {11},
	},
	"toxicity": {
		"retreat_fraction:bid":    {2, 4},
		"net_withdrawal_rate:bid": {4},
	},
	"hold": {
		"trade_rate_ratio":   {9},
		"bars_held":          {9},
		"giveback_from_peak": {3},
		"evaporation_index":  {4},
	},
}
