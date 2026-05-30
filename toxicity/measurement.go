package toxicity

import (
	"math"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/market/perspectives"
)

const noiseFloorSNR = 1.0

/*
bookQualitySnapshot is the per-symbol cancel/fill asymmetry the tracker maintains.
*/
type bookQualitySnapshot struct {
	cancelBid float64
	fillBid   float64
	cancelAsk float64
	fillAsk   float64
	toxicNear bool
}

/*
Measure classifies book-quality into toxicity perspective categories. SNR scales
the dominant asymmetry against MinFillToCancelRatio.
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

	snr := raw

	if snr < noiseFloorSNR {
		return perspectives.Measurement{}, false
	}

	return perspectives.Measurement{
		Source:   perspectives.SourceToxicity,
		Category: category,
		Strength: snr,
		SNR:      snr,
	}, true
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
			continue
		}

		if state.mid > 0 && math.Abs(price-state.mid)/state.mid <= toxicProximityPct {
			snapshot.toxicNear = true

			break
		}
	}

	return snapshot, true
}

func classifyBookQuality(snapshot bookQualitySnapshot) (perspectives.CategoryType, float64) {
	if snapshot.toxicNear {
		return perspectives.CategoryToxicBluff, 2.0
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
