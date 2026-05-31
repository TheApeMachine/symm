package toxicity

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

/*
bookQualitySnapshot is the per-symbol cancel/fill asymmetry the tracker maintains.
*/
type bookQualitySnapshot struct {
	cancelBid          float64
	fillBid            float64
	cancelAsk          float64
	fillAsk            float64
	toxicNear          bool
	toxicBluffStrength float64
}

/*
Measure classifies book-quality into toxicity perspective categories. Strength
holds the raw asymmetry; SNR is playbook sigma units via FinalizeMeasurement.
*/
func (tracker *Tracker) Measure(symbol string, at time.Time) (perspectives.Measurement, bool) {
	snapshot, ok := tracker.snapshot(symbol, at)

	if !ok {
		return perspectives.Measurement{}, false
	}

	category, raw := classifyBookQuality(snapshot)

	if category == perspectives.CategoryTypeNone || raw <= 0 {
		return perspectives.Measurement{}, false
	}

	measurement := perspectives.Measurement{
		Symbol:   symbol,
		Source:   perspectives.SourceToxicity,
		Category: category,
	}

	return perspectives.FinalizeMeasurement(measurement, raw, "book_quality"), true
}

func (tracker *Tracker) snapshot(symbol string, at time.Time) (bookQualitySnapshot, bool) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	state := tracker.symbols[symbol]

	if state == nil {
		return bookQualitySnapshot{}, false
	}

	snapshot := bookQualitySnapshot{
		cancelBid: state.cancelBid,
		fillBid:   state.fillBid,
		cancelAsk: state.cancelAsk,
		fillAsk:   state.fillAsk,
	}

	for price, expiry := range state.toxic {
		if at.After(expiry) {
			delete(state.toxicChurn, price)

			continue
		}

		if state.mid > 0 && math.Abs(price-state.mid)/state.mid <= toxicProximityPct {
			snapshot.toxicNear = true
			snapshot.toxicBluffStrength = math.Max(snapshot.toxicBluffStrength, state.toxicChurn[price])
		}
	}

	if snapshot.toxicNear && snapshot.toxicBluffStrength <= 0 {
		snapshot.toxicBluffStrength = 1
	}

	return snapshot, true
}

func (tracker *Tracker) flagToxicLocked(state *symbolState, price float64, churnRatio float64, now time.Time) {
	state.toxic[price] = now.Add(toxicCooldown)

	if churnRatio > 0 {
		state.toxicChurn[price] = churnRatio
	}
}

func classifyBookQuality(snapshot bookQualitySnapshot) (perspectives.CategoryType, float64) {
	if snapshot.toxicNear {
		strength := snapshot.toxicBluffStrength

		if strength < 1 {
			strength = 1
		}

		return perspectives.CategoryToxicBluff, strength
	}

	bidRatio := cancelFillRatio(snapshot.cancelBid, snapshot.fillBid)
	askRatio := cancelFillRatio(snapshot.cancelAsk, snapshot.fillAsk)
	threshold := config.System.MinFillToCancelRatio

	if bidRatio >= threshold || askRatio >= threshold {
		strength := math.Max(bidRatio, askRatio) / threshold

		return perspectives.CategoryLiquidityVacuum, strength
	}

	if bidRatio > 0 && askRatio > 0 &&
		bidRatio < threshold/2 && askRatio < threshold/2 {
		strength := (threshold/2 - math.Max(bidRatio, askRatio)) / (threshold / 2)

		return perspectives.CategoryHardSupport, strength
	}

	return perspectives.CategoryTypeNone, 0
}

func cancelFillRatio(cancel, fill float64) float64 {
	if cancel <= 0 {
		return 0
	}

	return cancel / (fill + epsilon)
}
