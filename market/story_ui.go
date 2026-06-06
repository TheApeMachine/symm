package market

import (
	"math"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives/types"
)

const (
	marketSymbol = "market"
)

var predictionDefaultBandEdges = []float64{0.75, 1.0, 1.5}

func thesisScoreRMS(snapshots []types.Measurement) float64 {
	sumSquares := 0.0
	count := 0

	for _, measurement := range snapshots {
		if measurement.SNR <= 0 || math.IsNaN(measurement.SNR) || math.IsInf(measurement.SNR, 0) {
			continue
		}

		sumSquares += measurement.SNR * measurement.SNR
		count++
	}

	if count == 0 {
		return 0
	}

	return math.Sqrt(sumSquares / float64(count))
}

func requiredEntryScore(snapshots []types.Measurement) float64 {
	multiple := viper.GetFloat64("trading.entry_edge_multiple")

	if multiple <= 0 {
		multiple = 2
	}

	return multiple * roundTripFrictionPct(snapshots) * 100
}

func roundTripFrictionPct(snapshots []types.Measurement) float64 {
	spreadBPS := 0.0

	for _, measurement := range snapshots {
		if measurement.SpreadBPS > spreadBPS {
			spreadBPS = measurement.SpreadBPS
		}
	}

	takerFeePct := viper.GetFloat64("trading.paper.taker_fee_pct")

	if takerFeePct <= 0 {
		takerFeePct = viper.GetFloat64("trading.taker_fee_pct")
	}

	return 2*takerFeePct + spreadBPS/100
}
