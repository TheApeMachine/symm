package pumpdump

import (
	"math"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/stats"
)

const (
	minVolumeHistory = 4
	minPriceHistory  = 3
	minSpreadHistory = 8
)

/*
Filter scores one symbol from stored track state and a live snapshot.
*/
type Filter interface {
	Score(
		symbol string,
		trackStore *TrackStore,
		snapshot engine.Snapshot,
		now time.Time,
	) (confidence float64, reason string)
}

/*
PrecursorFilter implements the default pump precursor gate chain.
*/
type PrecursorFilter struct{}

/*
Score applies liquidity, volume, book, and confirmation gates.
*/
func (filter *PrecursorFilter) Score(
	symbol string,
	trackStore *TrackStore,
	snapshot engine.Snapshot,
	now time.Time,
) (float64, string) {
	_ = now

	if !filter.passesLiquidity(symbol, trackStore) {
		return 0, ""
	}

	volumeRatio, volumeSpike := filter.volumeSpike(symbol, trackStore)

	if !snapshot.ImbalanceOK || !snapshot.PressureOK {
		return 0, ""
	}

	micro := precursorScore(snapshot.Imbalance, snapshot.BuyPressure)

	if micro <= 0 || volumeRatio <= 0 {
		return 0, ""
	}

	calibration := trackStore.CalibrationScale(symbol)

	if calibration <= 0 {
		return 0, ""
	}

	confidence := volumeRatio * micro * calibration
	reason := "precursor"

	if !volumeSpike {
		return 0, reason
	}

	if !filter.priceFlat(symbol, trackStore) {
		return 0, reason
	}

	if !snapshot.SpreadOK || !filter.spreadTight(symbol, trackStore, snapshot.SpreadBPS) {
		return 0, reason
	}

	return confidence, "actual_pump"
}

func (filter *PrecursorFilter) passesLiquidity(symbol string, trackStore *TrackStore) bool {
	track, ok := trackStore.bySymbol[symbol]

	if !ok || track.dailyQuoteVol <= 0 {
		return false
	}

	quoteVolumes := make([]float64, 0, len(trackStore.bySymbol))

	for _, candidate := range trackStore.bySymbol {
		if candidate.dailyQuoteVol <= 0 {
			continue
		}

		quoteVolumes = append(quoteVolumes, candidate.dailyQuoteVol)
	}

	if len(quoteVolumes) < 2 {
		return false
	}

	liquidityLine := stats.CrossSectionMedian(quoteVolumes)

	return track.dailyQuoteVol < liquidityLine
}

func (filter *PrecursorFilter) volumeSpike(
	symbol string,
	trackStore *TrackStore,
) (float64, bool) {
	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.volumes) < minVolumeHistory {
		return 0, false
	}

	baseline := stats.Mean(track.volumes)

	if baseline <= 0 || track.bucketVolume <= 0 {
		return 0, false
	}

	ratio := track.bucketVolume / baseline
	fence := volumeRatioFence(volumeRatios(track.volumes))

	if fence <= 0 {
		return 0, false
	}

	return ratio, ratio > fence
}

func (filter *PrecursorFilter) priceFlat(symbol string, trackStore *TrackStore) bool {
	track, ok := trackStore.bySymbol[symbol]

	if !ok || track.bucketOpenPrice <= 0 || track.lastPrice <= 0 {
		return false
	}

	if len(track.priceMoves) < minPriceHistory {
		return false
	}

	move := stats.AbsRelativeMove(track.lastPrice, track.bucketOpenPrice)
	quietLine := stats.Median(track.priceMoves)

	return move < quietLine
}

func (filter *PrecursorFilter) spreadTight(
	symbol string,
	trackStore *TrackStore,
	spreadBPS float64,
) bool {
	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.spreads) < minSpreadHistory {
		return false
	}

	medianSpread := stats.Median(track.spreads)

	return spreadBPS < medianSpread
}

func volumeRatios(volumes []float64) []float64 {
	baseline := stats.Mean(volumes)

	if baseline <= 0 {
		return nil
	}

	ratios := make([]float64, len(volumes))

	for index, volume := range volumes {
		ratios[index] = volume / baseline
	}

	return ratios
}

func volumeRatioFence(ratios []float64) float64 {
	if len(ratios) == 0 {
		return 0
	}

	lower, upper := stats.Quartiles(ratios)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return stats.Max(ratios)
}

func precursorScore(imbalance, buyPressure float64) float64 {
	if imbalance <= 0 || buyPressure <= 0 {
		return 0
	}

	bookSide := math.Min(imbalance, 1)
	buySide := (buyPressure + 1) / 2

	return bookSide * buySide
}
