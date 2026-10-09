package store

import (
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
Grid is not a grid at all, it is a "group by" over Measurement
and Metric pins into 12 multi-signal confluence regions.
*/
type Grid struct{}

func NewGrid() *Grid {
	return &Grid{}
}

/*
Observe sums the dampened metric values across each confluence tile
and returns the winning region token (e.g. []byte("R04") or byte 0x04).
*/
func (grid *Grid) Observe(measurement *data.Measurement) []byte {
	// Fixed stack arrays: 0 allocations! (Index 0 unused, 1..12 for R01..R12)
	var scores [13]float64
	var counts [13]int

	// 1. Process focal measurement metrics
	source := measurement.Source
	for metric := range measurement.Read() {
		region := grid.PinRegion(source, metric.Key)
		// Magnitude of deformation: pins push out whether positive or negative!
		scores[region] += math.Abs(metric.Metric.Standardized)
		counts[region]++
	}

	// 2. Process all peer signals joined on this measurement
	for _, peer := range measurement.Peers() {
		peerSource := peer.Source
		for metric := range peer.Read() {
			region := grid.PinRegion(peerSource, metric.Key)
			scores[region] += math.Abs(metric.Metric.Standardized)
			counts[region]++
		}
	}

	// 3. Find the brightest region using Stouffer's normalization (Score / sqrt(N))
	winningRegion := uint8(2) // Default to R02 (neutral clock)
	maxBrightness := 0.0

	for r := uint8(1); r <= 12; r++ {
		if counts[r] == 0 {
			continue
		}

		// Normalize by sqrt(counts) so 44 CVD metrics don't crush 9 Morphology metrics!
		brightness := scores[r] / math.Sqrt(float64(counts[r]))

		if brightness > maxBrightness {
			maxBrightness = brightness
			winningRegion = r
		}
	}

	// Return zero-padded token for S3 path compatibility: "R01" ... "R12"
	return fmt.Appendf(nil, "R%02d", winningRegion)
}

/*
PinRegion maps (source, metric) to its 12-tile confluence region.
*/
func (grid *Grid) PinRegion(source, metric string) uint8 {
	m := strings.ToLower(metric)
	s := strings.ToLower(source)

	switch s {
	case "hawkes", "level3":
		if strings.Contains(m, ":buy") || strings.Contains(m, "buy_from_buy") {
			return 1 // R01: Buy Arrival Cascade
		}

		if strings.Contains(m, ":sell") || strings.Contains(m, "sell_from_sell") {
			return 3 // R03: Sell Arrival Cascade
		}

		return 2 // R02: Clock Acceleration / Branching

	case "pumpdump":
		if strings.Contains(m, "positive") {
			return 1 // R01
		}

		if strings.Contains(m, "negative") {
			return 3 // R03
		}

		return 2 // R02

	case "cvd", "spot:trade":
		if strings.Contains(m, ":buy") || (strings.Contains(m, "signed_net_fraction") && !strings.Contains(m, "divergence")) {
			return 4 // R04: Aggressive Taker Buy Inflow
		}

		if strings.Contains(m, ":sell") {
			return 6 // R06: Aggressive Taker Sell Outflow
		}

		return 5 // R05: Gross Churn / Two-Sided Volume

	case "depthflow", "toxicity", "liquidity":
		// Asks retreating = Offer vacuum
		if strings.Contains(m, ":ask") && (strings.Contains(m, "retreat") || strings.Contains(m, "removed")) {
			return 7 // R07: Ask Void
		}

		// Bids retreating = Waterfall
		if strings.Contains(m, ":bid") && (strings.Contains(m, "retreat") || strings.Contains(m, "removed")) {
			return 9 // R09: Bid Pulling
		}

		return 8 // R08: Spread / Friction / Turnover

	case "morphology":
		return 11 // R11: Iceberg Absorption Wall / Geometric Distortion

	case "sentiment", "correlation", "leadlag":
		if strings.Contains(m, "advance") || strings.Contains(m, "consensus") {
			return 10 // R10: Cohort Advance
		}

		if strings.Contains(m, "decline") || strings.Contains(m, "dispersion") {
			return 12 // R12: Cohort Decline
		}

		return 11 // R11
	}

	return 2 // Default neutral clock
}
