package pumpdump

import (
	"math"
	"time"

	"github.com/theapemachine/symm/engine"
)

type volumeTick struct {
	at     time.Time
	volume float64
}

const (
	minVolumeHistory = 4
	minPriceHistory  = 3
	minSpreadHistory = 8
	volumeHistoryCap = 20
	bucketWindow     = 5 * time.Minute
	spreadHistoryCap = 20
	priceHistoryCap  = 20
)

/*
TrackStore holds per-symbol rolling windows for pump precursors.
*/
type TrackStore struct {
	shard    engine.ShardedStore
	bySymbol map[string]*SymbolTrack
}

/*
SymbolTrack accumulates rolling volume windows used by the article trigger.
*/
type SymbolTrack struct {
	engine.SymbolLock
	volumes           []float64
	spreads           []float64
	priceMoves        []float64
	confidenceHistory []float64
	rollingTicks      []volumeTick
	bucketVolume      float64
	bucketOpenPrice   float64
	lastPrice         float64
	dailyQuoteVol     float64
	bucketStart       time.Time
	lastRollAt        time.Time
	calibrator        engine.PredictionCalibrator
	liveScore         float64
}

/*
BeginScan clears per-tick live gauge scores before the next scan set runs.
*/
func (trackStore *TrackStore) BeginScan() {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	for _, track := range trackStore.bySymbol {
		track.liveScore = 0
	}
}

/*
NewTrackStore creates an empty track store.
*/
func NewTrackStore() *TrackStore {
	return &TrackStore{
		bySymbol: make(map[string]*SymbolTrack),
	}
}

/*
ApplyTicker ingests last price and 24h quote volume for liquidity filtering.
*/
func (trackStore *TrackStore) ApplyTicker(symbol string, last, volumeBase float64) {
	if symbol == "" || last <= 0 {
		return
	}

	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track := trackStore.ensure(symbol)
	track.lastPrice = last
	track.dailyQuoteVol = volumeBase * last
}

/*
AddVolume adds executed trade volume into the rolling window.
*/
func (trackStore *TrackStore) AddVolume(symbol string, volume float64, now time.Time) {
	if symbol == "" || volume <= 0 {
		return
	}

	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track := trackStore.ensure(symbol)
	track.rollingTicks = append(track.rollingTicks, volumeTick{at: now, volume: volume})
	track.pruneRolling(now)
	track.bucketVolume = track.rollingVolume()
}

/*
RecordSpread stores bid-ask spread samples for tightening detection.
*/
func (trackStore *TrackStore) RecordSpread(symbol string, spreadBPS float64) {
	if symbol == "" || spreadBPS <= 0 {
		return
	}

	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track := trackStore.ensure(symbol)
	track.spreads = append(track.spreads, spreadBPS)

	if len(track.spreads) > spreadHistoryCap {
		track.spreads = track.spreads[len(track.spreads)-spreadHistoryCap:]
	}
}

/*
RollBuckets closes any elapsed five-minute windows.
*/
func (trackStore *TrackStore) RollBuckets(now time.Time) {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	for _, track := range trackStore.bySymbol {
		track.roll(now)
		track.pruneRolling(now)
		track.bucketVolume = track.rollingVolume()
	}
}

/*
PassesLiquidity keeps symbols below the live cross-section median daily quote volume.
*/
func (trackStore *TrackStore) PassesLiquidity(symbol string) bool {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

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

	liquidityLine := crossSectionMedian(quoteVolumes)

	return track.dailyQuoteVol < liquidityLine
}

/*
ApplyPredictionFeedback updates precursor calibration from one settled forecast.
*/
func (trackStore *TrackStore) ApplyPredictionFeedback(feedback engine.PredictionFeedback) {
	if feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track := trackStore.ensure(feedback.Symbol)
	track.calibrator.Apply(feedback)
}

/*
FinalizeMeasurement normalizes raw confidence and derives the active bucket runway.
*/
func (trackStore *TrackStore) FinalizeMeasurement(
	symbol string, rawConfidence float64, now time.Time, reason string,
) (float64, float64, time.Duration, string) {
	if rawConfidence <= 0 {
		return 0, 0, 0, ""
	}

	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track, ok := trackStore.bySymbol[symbol]

	if !ok {
		return 0, 0, 0, ""
	}

	normalized := engine.NormalizeConfidence(rawConfidence, track.confidenceHistory)
	track.liveScore = normalized

	runway := track.pumpRunway(now)

	if runway <= 0 {
		return 0, 0, 0, ""
	}

	track.recordConfidence(rawConfidence)
	expectedReturn := track.expectedReturnOverRunway(runway)

	return normalized, expectedReturn, runway, reason
}

/*
PeakLiveConfidence returns the highest unit-scale score across all symbols.
*/
func (trackStore *TrackStore) PeakLiveConfidence() float64 {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	peak := 0.0

	for _, track := range trackStore.bySymbol {
		if track.liveScore > peak {
			peak = track.liveScore
		}
	}

	return peak
}

/*
PeakSymbolScore returns the symbol with the highest live score.
*/
func (trackStore *TrackStore) PeakSymbolScore() (string, float64) {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	bestSymbol := ""
	bestScore := 0.0

	for symbol, track := range trackStore.bySymbol {
		if track.liveScore <= bestScore {
			continue
		}

		bestScore = track.liveScore
		bestSymbol = symbol
	}

	return bestSymbol, bestScore
}

func (track *SymbolTrack) expectedReturnOverRunway(runway time.Duration) float64 {
	if len(track.priceMoves) < minPriceHistory || runway <= 0 {
		return 0
	}

	quietLine := median(track.priceMoves)

	return quietLine * (runway.Seconds() / bucketWindow.Seconds())
}

func (track *SymbolTrack) pumpRunway(now time.Time) time.Duration {
	if track.bucketStart.IsZero() {
		return 0
	}

	remaining := bucketWindow - now.Sub(track.bucketStart)

	if remaining <= 0 {
		return 0
	}

	return remaining
}

func (track *SymbolTrack) recordConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}

	track.confidenceHistory = append(track.confidenceHistory, confidence)

	if len(track.confidenceHistory) > volumeHistoryCap {
		track.confidenceHistory = track.confidenceHistory[len(track.confidenceHistory)-volumeHistoryCap:]
	}
}

/*
CalibrationScale returns the live precursor parameter multiplier for one symbol.
*/
func (trackStore *TrackStore) CalibrationScale(symbol string) float64 {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track, ok := trackStore.bySymbol[symbol]

	if !ok {
		return 1
	}

	return track.calibrator.Scale()
}

/*
VolumeSpike reports whether current bucket volume exceeds the symbol's own ratio fence.
*/
func (trackStore *TrackStore) VolumeSpike(symbol string) (float64, bool) {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.volumes) < minVolumeHistory {
		return 0, false
	}

	baseline := mean(track.volumes)

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

/*
PriceFlat reports whether the active bucket move is below the symbol's own move history.
*/
func (trackStore *TrackStore) PriceFlat(symbol string) bool {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || track.bucketOpenPrice <= 0 || track.lastPrice <= 0 {
		return false
	}

	if len(track.priceMoves) < minPriceHistory {
		return false
	}

	move := math.Abs(track.lastPrice/track.bucketOpenPrice - 1)
	quietLine := median(track.priceMoves)

	return move < quietLine
}

/*
SpreadTight reports sudden quote compression vs the spread history.
*/
func (trackStore *TrackStore) SpreadTight(symbol string, spreadBPS float64) bool {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.spreads) < minSpreadHistory {
		return false
	}

	medianSpread := median(track.spreads)

	return spreadBPS < medianSpread
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolTrack {
	return trackStore.ensureLocked(symbol)
}

func (trackStore *TrackStore) track(symbol string) *SymbolTrack {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	return trackStore.ensureLocked(symbol)
}

func (trackStore *TrackStore) ensureLocked(symbol string) *SymbolTrack {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolTrack{
		volumes:           make([]float64, 0, volumeHistoryCap),
		spreads:           make([]float64, 0, spreadHistoryCap),
		priceMoves:        make([]float64, 0, priceHistoryCap),
		confidenceHistory: make([]float64, 0, volumeHistoryCap),
		calibrator:        engine.NewPredictionCalibrator(),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func (track *SymbolTrack) roll(now time.Time) {
	if track.bucketStart.IsZero() {
		track.bucketStart = now
		track.bucketOpenPrice = track.lastPrice
		track.lastRollAt = now
		return
	}

	if now.Sub(track.bucketStart) < bucketWindow {
		return
	}

	if track.bucketOpenPrice > 0 && track.lastPrice > 0 {
		move := math.Abs(track.lastPrice/track.bucketOpenPrice - 1)
		track.priceMoves = append(track.priceMoves, move)

		if len(track.priceMoves) > priceHistoryCap {
			track.priceMoves = track.priceMoves[len(track.priceMoves)-priceHistoryCap:]
		}
	}

	closedVolume := track.rollingVolume()

	if closedVolume > 0 {
		track.volumes = append(track.volumes, closedVolume)

		if len(track.volumes) > volumeHistoryCap {
			track.volumes = track.volumes[len(track.volumes)-volumeHistoryCap:]
		}
	}

	track.bucketStart = now
	track.bucketOpenPrice = track.lastPrice
	track.lastRollAt = now
}

func (track *SymbolTrack) pruneRolling(now time.Time) {
	cutoff := now.Add(-bucketWindow)
	kept := track.rollingTicks[:0]

	for _, tick := range track.rollingTicks {
		if tick.at.Before(cutoff) {
			continue
		}

		kept = append(kept, tick)
	}

	track.rollingTicks = kept
}

func (track *SymbolTrack) rollingVolume() float64 {
	total := 0.0

	for _, tick := range track.rollingTicks {
		total += tick.volume
	}

	return total
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sum float64

	for _, value := range values {
		sum += value
	}

	return sum / float64(len(values))
}
