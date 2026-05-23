package trader

import "github.com/theapemachine/symm/config"

func batchPeakConfidence(candidates []tradeCandidate) float64 {
	peak := 0.0

	for _, candidate := range candidates {
		if candidate.confidence > peak {
			peak = candidate.confidence
		}
	}

	return peak
}

func entryWeight(confidence, peakConfidence float64) float64 {
	if confidence <= 0 || peakConfidence <= 0 {
		return 0
	}

	weight := confidence / peakConfidence

	if weight > 1 {
		return 1
	}

	return weight
}

func (crypto *Crypto) entryNotional(confidence, peakConfidence float64) float64 {
	if crypto.wallet.Balance <= 0 || config.System.MaxSlotPct <= 0 {
		return 0
	}

	weight := entryWeight(confidence, peakConfidence)

	if weight <= 0 {
		return 0
	}

	slotCap := crypto.wallet.Balance * config.System.MaxSlotPct / 100
	notional := slotCap * weight

	if config.System.MinCostEUR > 0 && notional < config.System.MinCostEUR {
		return 0
	}

	if notional > slotCap {
		return slotCap
	}

	return notional
}

func (crypto *Crypto) canAffordEntry(notional float64) bool {
	if notional <= 0 {
		return false
	}

	entryFee := config.System.TakerFee(notional, crypto.wallet.FeePct)

	return crypto.wallet.Balance >= notional+entryFee
}

func (crypto *Crypto) tradingSolvent() bool {
	equity, _ := crypto.markToMarket()

	return equity > config.System.MinCostEUR
}
