package fluid

import (
	"math"
	"time"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/ring"
)

const fieldHistoryCap = 64

/*
TrackStore holds per-symbol fluid field histories.
*/
type TrackStore struct {
	engine.GaugeScan
	shard             engine.ShardedStore
	bySymbol          map[string]*SymbolField
	calibrationParams engine.CalibrationParams
}

/*
SymbolField tracks density, velocity, spread, and confidence samples.
*/
type SymbolField struct {
	engine.SymbolLock
	samples           []fieldSample
	velocities        ring.FloatRing
	sourceHistory     ring.FloatRing
	shockHistory      ring.FloatRing
	confidenceHistory ring.FloatRing
	scratch           []float64
	dailyQuoteVol     float64
	lastPrice         float64
	lastSample        fieldSample
	lastAt            time.Time
	hasPrior          bool
	calibrator        engine.PredictionCalibrator
	liveScore         float64
}

/*
NewTrackStore creates an empty fluid track store with injected calibration parameters.
*/
func NewTrackStore(calibrationParams engine.CalibrationParams) *TrackStore {
	return &TrackStore{
		bySymbol:          make(map[string]*SymbolField),
		calibrationParams: calibrationParams,
	}
}

/*
BeginScan clears per-tick live gauge scores before the next scan set runs.
*/
func (trackStore *TrackStore) BeginScan() {
	trackStore.ResetGaugeScan()

	trackStore.shard.RLockMap()
	tracks := make([]*SymbolField, 0, len(trackStore.bySymbol))

	for _, track := range trackStore.bySymbol {
		tracks = append(tracks, track)
	}

	trackStore.shard.RUnlockMap()

	for _, track := range tracks {
		track.Lock()
		track.liveScore = 0
		track.Unlock()
	}
}

/*
ApplyTicker ingests 24h quote volume for liquidity filtering.
*/
func (trackStore *TrackStore) ApplyTicker(symbol string, last, volumeBase float64) {
	if symbol == "" || last <= 0 {
		return
	}

	track := trackStore.track(symbol)
	track.Lock()
	defer track.Unlock()

	track.dailyQuoteVol = volumeBase * last
}

/*
ApplyPredictionFeedback updates field-parameter calibration from one settled forecast.
*/
func (trackStore *TrackStore) ApplyPredictionFeedback(feedback engine.PredictionFeedback) {
	if feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	track := trackStore.track(feedback.Symbol)
	track.Lock()
	defer track.Unlock()

	track.calibrator.Apply(feedback)
}

/*
Sample ingests one fluid field observation for a symbol.
*/
func (trackStore *TrackStore) Sample(
	symbol string,
	density, price, spreadBPS, depthSlope, flow, buyPressure float64,
	now time.Time,
) (float64, string) {
	track := trackStore.track(symbol)
	track.Lock()
	defer track.Unlock()

	if track.hasPrior && !now.After(track.lastAt) {
		return 0, ""
	}

	current := fieldSample{
		density:   density,
		viscosity: viscosityFromDepth(spreadBPS, depthSlope),
		flow:      flow,
	}

	if track.hasPrior && !track.lastAt.IsZero() {
		elapsed := now.Sub(track.lastAt).Seconds()

		if elapsed > 0 && track.lastPrice > 0 && price > 0 {
			current.velocity = (price - track.lastPrice) / track.lastPrice / elapsed
		}
	}

	if !track.hasPrior {
		track.lastPrice = price
		track.lastSample = current
		track.lastAt = now
		track.hasPrior = true

		return 0, ""
	}

	source := continuitySource(current, track.lastSample)
	shock := burgersShock(current, track.lastSample)
	calibration := track.calibrator.Scale()

	if calibration <= 0 {
		return 0, ""
	}

	source *= calibration
	shock *= calibration

	sourceFence := ratioFence(track.ringScratch(track.sourceHistory))
	shockFence := ratioFence(track.ringScratch(track.shockHistory))

	quiet := quietVelocity(track.ringScratch(track.velocities), current.velocity)
	accumulating := quiet && sourceFence > 0 && source > sourceFence
	shocking := shockFence > 0 && shock > shockFence

	if source > 0 {
		track.sourceHistory.Push(source)
	}

	if shock > 0 {
		track.shockHistory.Push(shock)
	}

	track.velocities.Push(current.velocity)
	track.samples = append(track.samples, current)

	if len(track.samples) > fieldHistoryCap {
		track.samples = track.samples[len(track.samples)-fieldHistoryCap:]
	}

	track.lastPrice = price
	track.lastSample = current
	track.lastAt = now

	if !accumulating && !shocking {
		return 0, ""
	}

	rawConfidence := fieldConfidence(source, shock, buyPressure, quiet)

	if rawConfidence <= 0 {
		return 0, ""
	}

	reason := "accumulation"

	if shocking && !accumulating {
		reason = "shock"
	}

	normalized := track.calibrator.NormalizeConfidence(rawConfidence, track.ringScratch(track.confidenceHistory))
	track.recordConfidence(rawConfidence)
	track.liveScore = normalized

	return normalized, reason
}

/*
SymbolLiveScore returns the latest normalized gauge reading for one symbol.
*/
func (trackStore *TrackStore) SymbolLiveScore(symbol string) float64 {
	track := trackStore.track(symbol)
	track.Lock()
	defer track.Unlock()

	return track.liveScore
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

	return track.dailyQuoteVol < crossSectionMedian(quoteVolumes)
}

/*
SampledCount returns symbols with at least one stored sample.
*/
func (trackStore *TrackStore) SampledCount() int {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	count := 0

	for _, track := range trackStore.bySymbol {
		if len(track.samples) > 0 {
			count++
		}
	}

	return count
}

/*
WarmingCount returns symbols with ticker volume but no samples yet.
*/
func (trackStore *TrackStore) WarmingCount() int {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	count := 0

	for _, track := range trackStore.bySymbol {
		if track.dailyQuoteVol > 0 && len(track.samples) == 0 {
			count++
		}
	}

	return count
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

/*
PeakFieldSymbol returns the symbol with the strongest recent field activity.
Used when the live gauge score comes from cross-section field state.
*/
func (trackStore *TrackStore) PeakFieldSymbol() string {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	bestSymbol := ""
	bestActivity := 0.0

	for symbol, track := range trackStore.bySymbol {
		if len(track.samples) == 0 {
			continue
		}

		sample := track.samples[len(track.samples)-1]
		activity := math.Abs(sample.velocity)*sample.density + sample.density

		if activity <= bestActivity {
			continue
		}

		bestActivity = activity
		bestSymbol = symbol
	}

	return bestSymbol
}

func (track *SymbolField) recordConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}

	track.confidenceHistory.Push(confidence)
}

func (track *SymbolField) ringScratch(history ring.FloatRing) []float64 {
	count := history.Len()

	if count == 0 {
		return nil
	}

	if cap(track.scratch) < count {
		track.scratch = make([]float64, count)
	}

	scratch := track.scratch[:count]

	for index := 0; index < count; index++ {
		scratch[index] = history.At(index)
	}

	return scratch
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolField {
	return trackStore.ensureLocked(symbol)
}

func (trackStore *TrackStore) track(symbol string) *SymbolField {
	trackStore.shard.LockMap()
	defer trackStore.shard.UnlockMap()

	return trackStore.ensureLocked(symbol)
}

func (trackStore *TrackStore) ensureLocked(symbol string) *SymbolField {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolField{
		samples:           make([]fieldSample, 0, fieldHistoryCap),
		velocities:        ring.NewFloatRing(fieldHistoryCap),
		sourceHistory:     ring.NewFloatRing(fieldHistoryCap),
		shockHistory:      ring.NewFloatRing(fieldHistoryCap),
		confidenceHistory: ring.NewFloatRing(fieldHistoryCap),
		scratch:           make([]float64, 0, fieldHistoryCap),
		calibrator:        engine.NewPredictionCalibrator(trackStore.calibrationParams),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func viscosityFromDepth(spreadBPS, depthSlope float64) float64 {
	if spreadBPS <= 0 {
		return 0
	}

	if depthSlope <= 0 {
		return spreadBPS
	}

	return spreadBPS / (1 + depthSlope)
}
