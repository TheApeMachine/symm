package fluid

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/engine"
)

const fieldHistoryCap = 64

/*
TrackStore holds per-symbol fluid field histories.
*/
type TrackStore struct {
	mu       sync.Mutex
	bySymbol map[string]*SymbolField
}

/*
SymbolField tracks density, velocity, spread, and confidence samples.
*/
type SymbolField struct {
	samples           []fieldSample
	velocities        []float64
	sourceHistory     []float64
	shockHistory      []float64
	confidenceHistory []float64
	dailyQuoteVol     float64
	lastPrice         float64
	lastSample        fieldSample
	lastAt            time.Time
	hasPrior          bool
	calibrator        engine.PredictionCalibrator
	liveScore         float64
}

/*
NewTrackStore creates an empty fluid track store.
*/
func NewTrackStore() *TrackStore {
	return &TrackStore{
		bySymbol: make(map[string]*SymbolField),
	}
}

/*
BeginScan clears per-tick live gauge scores before the next scan set runs.
*/
func (trackStore *TrackStore) BeginScan() {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	for _, track := range trackStore.bySymbol {
		track.liveScore = 0
	}
}

/*
ApplyTicker ingests 24h quote volume for liquidity filtering.
*/
func (trackStore *TrackStore) ApplyTicker(symbol string, last, volumeBase float64) {
	if symbol == "" || last <= 0 {
		return
	}

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	track.dailyQuoteVol = volumeBase * last
}

/*
ApplyPredictionFeedback updates field-parameter calibration from one settled forecast.
*/
func (trackStore *TrackStore) ApplyPredictionFeedback(feedback engine.PredictionFeedback) {
	if feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(feedback.Symbol)
	track.calibrator.Apply(feedback)
}

/*
Sample ingests one fluid field observation for a symbol.
*/
func (trackStore *TrackStore) Sample(
	symbol string,
	density, price, spreadBPS, flow, buyPressure float64,
	now time.Time,
) (float64, float64, time.Duration, string) {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)

	current := fieldSample{
		density:   density,
		viscosity: spreadBPS,
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

		return 0, 0, 0, ""
	}

	elapsed := now.Sub(track.lastAt).Seconds()
	source := continuitySource(current, track.lastSample)
	shock := burgersShock(current, track.lastSample)
	calibration := track.calibrator.Scale()

	if calibration <= 0 {
		return 0, 0, 0, ""
	}

	source *= calibration
	shock *= calibration

	sourceFence := ratioFence(track.sourceHistory)
	shockFence := ratioFence(track.shockHistory)

	quiet := quietVelocity(track.velocities, current.velocity)
	accumulating := quiet && sourceFence > 0 && source > sourceFence
	shocking := shockFence > 0 && shock > shockFence

	if source > 0 {
		track.sourceHistory = append(track.sourceHistory, source)
	}

	if shock > 0 {
		track.shockHistory = append(track.shockHistory, shock)
	}

	track.trimHistories()
	track.velocities = append(track.velocities, current.velocity)
	track.samples = append(track.samples, current)

	if len(track.velocities) > fieldHistoryCap {
		track.velocities = track.velocities[len(track.velocities)-fieldHistoryCap:]
	}

	if len(track.samples) > fieldHistoryCap {
		track.samples = track.samples[len(track.samples)-fieldHistoryCap:]
	}

	track.lastPrice = price
	track.lastSample = current
	track.lastAt = now

	if !accumulating && !shocking {
		return 0, 0, 0, ""
	}

	rawConfidence := 0.0
	reason := ""

	if accumulating {
		rawConfidence += source * buyPressure
		reason = "accumulation"
	}

	if shocking {
		rawConfidence += shock * buyPressure

		if reason == "" {
			reason = "shock"
		}
	}

	if rawConfidence <= 0 {
		return 0, 0, 0, ""
	}

	runway := fieldRunway(spreadBPS, current.velocity, elapsed)
	normalized := engine.NormalizeConfidence(rawConfidence, track.confidenceHistory)
	track.recordConfidence(rawConfidence)
	track.liveScore = normalized
	expectedReturn := current.velocity * runway.Seconds()

	if math.Abs(expectedReturn) <= 0 && spreadBPS > 0 && buyPressure > 0 {
		expectedReturn = (spreadBPS / 10000) * buyPressure
	}

	return normalized, expectedReturn, runway, reason
}

/*
PassesLiquidity keeps symbols below the live cross-section median daily quote volume.
*/
func (trackStore *TrackStore) PassesLiquidity(symbol string) bool {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

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
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

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
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

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
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

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
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

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

func (track *SymbolField) trimHistories() {
	if len(track.sourceHistory) > fieldHistoryCap {
		track.sourceHistory = track.sourceHistory[len(track.sourceHistory)-fieldHistoryCap:]
	}

	if len(track.shockHistory) > fieldHistoryCap {
		track.shockHistory = track.shockHistory[len(track.shockHistory)-fieldHistoryCap:]
	}
}

func (track *SymbolField) recordConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}

	track.confidenceHistory = append(track.confidenceHistory, confidence)

	if len(track.confidenceHistory) > fieldHistoryCap {
		track.confidenceHistory = track.confidenceHistory[len(track.confidenceHistory)-fieldHistoryCap:]
	}
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolField {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolField{
		samples:           make([]fieldSample, 0, fieldHistoryCap),
		velocities:        make([]float64, 0, fieldHistoryCap),
		sourceHistory:     make([]float64, 0, fieldHistoryCap),
		shockHistory:      make([]float64, 0, fieldHistoryCap),
		confidenceHistory: make([]float64, 0, fieldHistoryCap),
		calibrator:        engine.NewPredictionCalibrator(),
	}
	trackStore.bySymbol[symbol] = track

	return track
}
