package economics

import (
	"math"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
AdverseSelectionBPS scales the configured maker adverse-selection penalty by
toxicity SNR when a toxicity measurement is present.
*/
func AdverseSelectionBPS(measurements []perspectives.Measurement) float64 {
	penalty := config.System.AdverseSelectionBPS

	for _, measurement := range measurements {
		if measurement.Source != perspectives.SourceToxicity || measurement.SNR <= 0 {
			continue
		}

		return penalty * measurement.SNR
	}

	return penalty
}

/*
StressQuote perturbs a quote for pessimistic paper execution simulation.
Latency scales with the current micro-regime when regime is non-zero.
*/
func StressQuote(quote broker.Quote, adverseSelectionBPS float64, regime broker.StressRegime) broker.Quote {
	if !config.System.ExecutionStressEnabled {
		return quote
	}

	stressed := quote
	latency := broker.GlobalStressMachine().LatencyPenalty(config.System.ExecutionStressLatency, regime)

	if latency > 0 && !stressed.At.IsZero() {
		stressed.At = stressed.At.Add(-latency)
	}

	fraction := config.System.ExecutionStressDepthFraction

	if fraction > 0 && fraction < 1 {
		stressed.BidDepth = scaleDepth(stressed.BidDepth, fraction)
		stressed.AskDepth = scaleDepth(stressed.AskDepth, fraction)
	}

	if adverseSelectionBPS > 0 && stressed.Ask > 0 {
		bump := stressed.Ask * adverseSelectionBPS / 10000
		stressed.Ask += bump
		stressed.Last = math.Max(stressed.Last, stressed.Ask)
	}

	return stressed
}

/*
ShouldReject returns an error when stress-mode simulates an exchange reject.
*/
func ShouldReject(regime broker.StressRegime) error {
	if !config.System.ExecutionStressEnabled {
		return nil
	}

	rate := broker.EffectiveRejectRate(config.System.ExecutionStressRejectRate, regime)

	if rate <= 0 {
		return nil
	}

	return broker.GlobalStressMachine().RejectOutcome(rate, regime)
}

func scaleDepth(levels []market.BookLevel, fraction float64) []market.BookLevel {
	if len(levels) == 0 {
		return levels
	}

	scaled := make([]market.BookLevel, len(levels))

	for index, level := range levels {
		scaled[index] = market.BookLevel{
			Price: level.Price,
			Qty:   level.Qty * fraction,
		}
	}

	return scaled
}
