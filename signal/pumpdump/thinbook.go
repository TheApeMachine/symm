package pumpdump

import (
	"math"

	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/statutil"
)

/*
thinBookScore detects the TITCOIN thin-book trap: a huge % move on tiny USD
volume with a hollow, wide-spread book. Two pump species must stay separate —
a vertical ignition (UNFI/SRM/SLX) has executed volume AND book structure; a
thin-book pump is a wide spread over a hollow book on bottom-of-cross-section
dollar volume. Every threshold is derived (peer dollar-volume rank, spread vs
the symbol's own median, depth vs its own median) — no hardcoded "10%".

The score is in [0,1]: 1 is a fully disqualified trap, 0 is a structured book.
It is the product of three derived tells so a real pump (cheap on any single
axis) is not disqualified — all three must be thin together.
*/
func thinBookScore(
	sample tickerSample,
	book bookSnapshot,
	history measurementHistory,
	crossSection *market.CrossSection,
) float64 {
	volumeRank := dollarVolumeRank(sample, crossSection)

	if volumeRank <= 0 {
		return 0
	}

	spread := book.spread

	if spread <= 0 {
		spread = sample.spread
	}

	spreadWidth := spreadAnomaly(spread, history)
	hollowness := depthHollowness(book.touchDepth, history)

	return volumeRank * spreadWidth * hollowness
}

/*
dollarVolumeRank scores how far into the bottom of the cross-section the symbol's
dollar volume sits. The bottom quartile of peer dollar volume is the threshold;
a symbol below it ramps from 0 (at the quartile) to 1 (at zero volume).
*/
func dollarVolumeRank(sample tickerSample, crossSection *market.CrossSection) float64 {
	if crossSection == nil {
		return 0
	}

	peers := positiveSamples(crossSection.DollarVolumes())

	if len(peers) < 2 {
		return 0
	}

	lowerQuartile, _, err := statutil.Quartiles(peers)

	if err != nil || lowerQuartile <= 0 {
		return 0
	}

	dollarVolume := sample.volume * sample.last

	if dollarVolume >= lowerQuartile {
		return 0
	}

	return (lowerQuartile - dollarVolume) / lowerQuartile
}

/*
spreadAnomaly scores how many of the symbol's own median spreads wide the current
spread is. A spread at or below its median is structured (0); wider ramps toward
1 saturating one median above the baseline.
*/
func spreadAnomaly(spread float64, history measurementHistory) float64 {
	baseline := statutil.Median(positiveSamples(history.bookSpreads))

	if baseline <= 0 {
		baseline = statutil.Median(positiveSamples(history.spreads))
	}

	if baseline <= 0 || spread <= baseline {
		return 0
	}

	return math.Min(1, (spread-baseline)/baseline)
}

/*
depthHollowness scores how far below its own median touch depth the book sits. A
depth at or above its median is structured (0); a hollow book ramps toward 1 as
depth falls to zero.
*/
func depthHollowness(touchDepth float64, history measurementHistory) float64 {
	baseline := statutil.Median(positiveSamples(history.touchDepths))

	if baseline <= 0 || touchDepth <= 0 {
		return 0
	}

	if touchDepth >= baseline {
		return 0
	}

	return (baseline - touchDepth) / baseline
}
