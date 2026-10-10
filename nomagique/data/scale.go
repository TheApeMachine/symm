package data

import "strings"

/*
Scale is the space a metric stream is standardized in.
*/
type Scale uint8

const (
	// ScaleLinear standardizes the raw value.
	ScaleLinear Scale = iota
	// ScaleLog standardizes ln(raw) for a strictly positive multiplicative
	// quantity; a non-positive raw has no logarithm, so its z-score is
	// undefined and it does not enter the stream.
	ScaleLog
	// ScaleLogModulus standardizes a signed multiplicative quantity on the
	// core.LogModulus scale: sign kept, magnitude in logs of the stream's own
	// geometric-mean magnitude, zero at zero.
	ScaleLogModulus
)

/*
metricScales declares the streams that are not standardized linearly: rates
and velocities over a single venue interval, and Hawkes parameters that jump at
each refit. Their magnitudes span orders of magnitude, so a linear z-score
reports a long-tailed spread (a 5 microsecond gap against a 10 second one) as
a 1e7 sigma event. The same multiplicative treatment pumpdump gives its
notional rate. Labels are matched without their ':' scope or '@' peer.
*/
var metricScales = map[string]Scale{
	// cvd: per-trade rates (positive) and their velocities (signed).
	"trade_rate":                   ScaleLog,
	"gross_notional_rate":          ScaleLog,
	"buy_notional_rate":            ScaleLog,
	"sell_notional_rate":           ScaleLog,
	"gross_notional_rate_baseline": ScaleLog,
	"gross_notional_rate_ratio":    ScaleLog,
	"net_notional_rate":            ScaleLogModulus,
	"gross_notional_rate_velocity": ScaleLogModulus,
	"net_notional_rate_velocity":   ScaleLogModulus,
	"midpoint_return_rate":         ScaleLogModulus,

	// pumpdump: per-bar rates (positive) and their velocities (signed).
	"volume_rate":            ScaleLog,
	"notional_rate":          ScaleLog,
	"notional_rate_baseline": ScaleLog,
	"notional_rate_ratio":    ScaleLog,
	"notional_rate_velocity": ScaleLogModulus,

	// toxicity: per-bracket rates (positive) and fraction velocities.
	"touch_fill_rate":              ScaleLog,
	"net_withdrawal_rate":          ScaleLog,
	"net_replenishment_rate":       ScaleLog,
	"retreat_rate":                 ScaleLog,
	"fill_fraction_velocity":       ScaleLogModulus,
	"withdrawal_fraction_velocity": ScaleLogModulus,

	// liquidity: divergence velocities per touch interval.
	"divergence_velocity":        ScaleLogModulus,
	"spread_divergence_velocity": ScaleLogModulus,

	// depthflow: per-update flow rates.
	"added_notional_rate":            ScaleLog,
	"removed_notional_rate":          ScaleLog,
	"book_turnover_rate":             ScaleLog,
	"net_displayed_flow_rate":        ScaleLogModulus,
	"net_book_change_rate":           ScaleLogModulus,
	"signed_net_displayed_flow_rate": ScaleLogModulus,

	// correlation: energy rates and their ratio.
	"peer_return_energy_rate":         ScaleLog,
	"relative_return_energy":          ScaleLog,
	"relative_return_energy_baseline": ScaleLog,
	"relative_return_energy_velocity": ScaleLogModulus,

	// hawkes: intensities and the parameters each refit replaces.
	"conditional_intensity":          ScaleLog,
	"excitation_intensity":           ScaleLog,
	"background_rate":                ScaleLog,
	"excitation_amplitude":           ScaleLog,
	"excitation_decay":               ScaleLog,
	"excitation_timescale":           ScaleLog,
	"offspring":                      ScaleLog,
	"branching_spectral_radius":      ScaleLog,
	"expected_descendants_from_buy":  ScaleLog,
	"expected_descendants_from_sell": ScaleLog,
}

/*
CanonicalScale returns the space label's stream is standardized in.
*/
func CanonicalScale(label string) Scale {
	base := label

	if at := strings.IndexByte(base, '@'); at != -1 {
		base = base[:at]
	}

	if colon := strings.IndexByte(base, ':'); colon != -1 {
		base = base[:colon]
	}

	return metricScales[base]
}
