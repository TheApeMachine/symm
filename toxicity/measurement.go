package toxicity

import (
	"math"
	"time"

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
	bidDepth           float64
	askDepth           float64
	toxicNear          bool
	toxicBluffStrength float64
}

/*
Measure classifies book-quality into toxicity perspective categories. Strength
holds the raw asymmetry; Confidence is band margin at the moment of selection;
SNR is adaptive sigma of Confidence against this symbol's own history.
*/
func (tracker *Tracker) Measure(symbol string, at time.Time) (perspectives.Measurement, error) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	state := tracker.symbols[symbol]

	if state == nil {
		return perspectives.Measurement{}, nil
	}

	snapshot := bookQualitySnapshotLocked(state, at)
	category, strength, evidence := classifyBookQuality(tracker, snapshot)

	if category == perspectives.CategoryTypeNone || strength <= 0 || evidence <= 0 {
		return perspectives.Measurement{}, nil
	}

	if math.IsNaN(strength) || math.IsInf(strength, 0) {
		return perspectives.Measurement{}, nil
	}

	confidence, err := state.tracked.Observe(category, evidence, evidence)

	if err != nil {
		return perspectives.Measurement{}, err
	}

	if confidence <= 0 {
		return perspectives.Measurement{}, nil
	}

	if math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return perspectives.Measurement{}, nil
	}

	standout := evidence

	snr, err := perspectives.ScoreCategorySNR(tracker.floor, symbol, standout)

	if err != nil {
		return perspectives.Measurement{}, err
	}

	if state.lastPrice <= 0 {
		return perspectives.Measurement{}, nil
	}

	return perspectives.Measurement{
		Symbol:     symbol,
		Source:     perspectives.SourceToxicity,
		Category:   category,
		Strength:   strength,
		Confidence: confidence,
		SNR:        snr,
		Last:       state.lastPrice,
	}, nil
}

func bookQualitySnapshotLocked(state *symbolState, at time.Time) bookQualitySnapshot {
	snapshot := bookQualitySnapshot{
		cancelBid: state.cancelBid,
		fillBid:   state.fillBid,
		cancelAsk: state.cancelAsk,
		fillAsk:   state.fillAsk,
		bidDepth:  state.bidTotal,
		askDepth:  state.askTotal,
	}

	for price, expiry := range state.toxic {
		if at.After(expiry) {
			delete(state.toxic, price)
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

	return snapshot
}

func (tracker *Tracker) flagToxicLocked(state *symbolState, price float64, churnRatio float64, now time.Time) {
	state.toxic[price] = now.Add(toxicCooldown)

	if churnRatio > 0 {
		state.toxicChurn[price] = churnRatio
	}
}

func classifyBookQuality(
	tracker *Tracker, snapshot bookQualitySnapshot,
) (perspectives.CategoryType, float64, float64) {
	if snapshot.toxicNear {
		evidence := toxicBluffEvidence(snapshot.toxicBluffStrength)

		return perspectives.CategoryToxicBluff, snapshot.toxicBluffStrength, evidence
	}

	bidRatio := cancelFillRatio(snapshot.cancelBid, snapshot.fillBid)
	askRatio := cancelFillRatio(snapshot.cancelAsk, snapshot.fillAsk)
	maxRatio := math.Max(bidRatio, askRatio)

	if snapshot.bidDepth > 0 && snapshot.askDepth > 0 && maxRatio == 0 {
		depthBalance := math.Min(snapshot.bidDepth, snapshot.askDepth) /
			math.Max(snapshot.bidDepth, snapshot.askDepth)

		return perspectives.CategoryHardSupport, depthBalance, depthBalance
	}

	threshold := tracker.fillToCancelThreshold()

	if threshold <= 0 {
		return perspectives.CategoryTypeNone, 0, 0
	}

	bidVacuum := bidRatio >= threshold && snapshot.fillBid > 0
	askVacuum := askRatio >= threshold && snapshot.fillAsk > 0

	if bidVacuum || askVacuum {
		margin := maxRatio - threshold
		evidence := margin / (margin + threshold)
		strength := maxRatio / threshold

		return perspectives.CategoryLiquidityVacuum, strength, evidence
	}

	if bidRatio > 0 && askRatio > 0 &&
		bidRatio < threshold/2 && askRatio < threshold/2 {
		half := threshold / 2
		margin := half - maxRatio
		evidence := margin / half
		strength := evidence

		return perspectives.CategoryHardSupport, strength, evidence
	}

	return perspectives.CategoryTypeNone, 0, 0
}

func toxicBluffEvidence(churnRatio float64) float64 {
	if churnRatio <= flashChurnRatioThreshold {
		return 1
	}

	margin := churnRatio - flashChurnRatioThreshold
	span := 1 - flashChurnRatioThreshold

	return margin / (margin + span)
}

func cancelFillRatio(cancel, fill float64) float64 {
	if cancel <= 0 || fill <= 0 {
		return 0
	}

	return cancel / fill
}
