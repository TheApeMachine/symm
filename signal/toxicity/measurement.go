package toxicity

import (
	"math"
	"time"

	"github.com/theapemachine/symm/market/perspectives/types"
)

// vacuumStrengthCap bounds the cancel/fill asymmetry strength (see
// classifyBookQuality): the fill EMA decays toward zero through cancel-only
// stretches, so the uncapped ratio compounds without bound.
const vacuumStrengthCap = 100.0

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
SNR is the Z-scored Shannon surprisal of the selected category against this
symbol's running categorical prior.
*/
func (tracker *Tracker) Measure(symbol string, at time.Time) (types.Measurement, error) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	state := tracker.symbols[symbol]

	if state == nil {
		return types.Measurement{}, nil
	}

	snapshot := bookQualitySnapshotLocked(state, at)
	category, strength, evidence := classifyBookQuality(tracker, snapshot)
	strength = types.AdjustSourceValue(types.SourceToxicity, strength) // top-down prediction feedback

	if category == types.CategoryTypeNone || strength <= 0 || evidence <= 0 {
		return types.Measurement{}, nil
	}

	if math.IsNaN(strength) || math.IsInf(strength, 0) {
		return types.Measurement{}, nil
	}

	if err := state.tracked.Observe(category, evidence); err != nil {
		return types.Measurement{}, err
	}

	score, err := tracker.surpriseField.ScoreState(symbol, category)

	if err != nil {
		return types.Measurement{}, err
	}

	if state.lastPrice <= 0 {
		return types.Measurement{}, nil
	}

	return types.Measurement{
		Symbol:     symbol,
		Source:     types.SourceToxicity,
		Category:   category,
		Strength:   strength,
		Confidence: score.StabilizeConfidence(evidence),
		SNR:        score.SNR,
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

	for key, expiry := range state.toxic {
		if at.After(expiry) {
			delete(state.toxic, key)
			delete(state.toxicChurn, key)
			delete(state.toxicEvidence, key)

			continue
		}

		price := priceFromKey(key, state.pair)

		if state.mid > 0 && math.Abs(price-state.mid)/state.mid <= toxicProximityPct {
			snapshot.toxicNear = true
			snapshot.toxicBluffStrength = math.Max(
				snapshot.toxicBluffStrength,
				math.Max(state.toxicChurn[key], state.toxicEvidence[key]),
			)
		}
	}

	return snapshot
}

func (tracker *Tracker) flagToxicLocked(
	state *symbolState,
	price float64,
	churnRatio float64,
	evidence float64,
	now time.Time,
) {
	key := priceKey(price, state.pair)
	state.toxic[key] = now.Add(toxicCooldown)

	if churnRatio > 0 {
		state.toxicChurn[key] = churnRatio
	}

	if evidence > 0 {
		state.toxicEvidence[key] = evidence
	}
}

func classifyBookQuality(
	tracker *Tracker, snapshot bookQualitySnapshot,
) (types.CategoryType, float64, float64) {
	if snapshot.toxicNear {
		evidence := toxicBluffEvidence(snapshot.toxicBluffStrength)

		return types.CategoryToxicBluff, snapshot.toxicBluffStrength, evidence
	}

	bidRatio := cancelFillRatio(snapshot.cancelBid, snapshot.fillBid)
	askRatio := cancelFillRatio(snapshot.cancelAsk, snapshot.fillAsk)
	maxRatio := math.Max(bidRatio, askRatio)

	if snapshot.bidDepth > 0 && snapshot.askDepth > 0 && maxRatio == 0 {
		depthBalance := math.Min(snapshot.bidDepth, snapshot.askDepth) /
			math.Max(snapshot.bidDepth, snapshot.askDepth)
		evidence := types.UnitMagnitudeMargin(depthBalance)

		return types.CategoryHardSupport, depthBalance, evidence
	}

	threshold := tracker.fillToCancelThreshold()

	if threshold <= 0 {
		return types.CategoryTypeNone, 0, 0
	}

	bidVacuum := bidRatio >= threshold && snapshot.fillBid > 0
	askVacuum := askRatio >= threshold && snapshot.fillAsk > 0

	if bidVacuum || askVacuum {
		margin := maxRatio - threshold
		evidence := types.UnitCompetitionMargin(margin, threshold)
		// Fill flow is an EMA decaying toward zero through any cancel-only
		// stretch, so the raw cancel/fill ratio compounds without bound —
		// observed at 2.5e+150 on ZEC/EUR, which then overflowed the
		// diagnostics stats into +Inf. A vacuum at the cap is already
		// saturated information.
		strength := math.Min(maxRatio/threshold, vacuumStrengthCap)

		return types.CategoryLiquidityVacuum, strength, evidence
	}

	if bidRatio > 0 && askRatio > 0 &&
		bidRatio < threshold/2 && askRatio < threshold/2 {
		half := threshold / 2
		margin := half - maxRatio
		evidence := types.UnitCompetitionMargin(margin, half)
		strength := evidence

		return types.CategoryHardSupport, strength, evidence
	}

	return types.CategoryTypeNone, 0, 0
}

func toxicBluffEvidence(churnRatio float64) float64 {
	if churnRatio <= 0 {
		return 0
	}

	if churnRatio <= flashChurnRatioThreshold {
		return types.UnitMagnitudeMargin(churnRatio)
	}

	margin := churnRatio - flashChurnRatioThreshold
	span := 1 - flashChurnRatioThreshold

	return types.UnitCompetitionMargin(margin, span)
}

func cancelFillRatio(cancel, fill float64) float64 {
	if cancel <= 0 || fill <= 0 {
		return 0
	}

	return cancel / fill
}
