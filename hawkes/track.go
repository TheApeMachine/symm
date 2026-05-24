package hawkes

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/engine"
)

/*
TrackStore holds per-symbol Hawkes confidence history and liquidity state.
*/
type TrackStore struct {
	mu       sync.Mutex
	bySymbol map[string]*SymbolTrack
}

/*
SymbolTrack stores rolling confidence samples, daily quote volume, and warm-start fits.
*/
type SymbolTrack struct {
	confidenceHistory []float64
	intensityRatios   []float64
	dailyQuoteVol     float64
	fit               BivariateFit
	hasFit            bool
	minFitEvents      int
	calibrator        engine.PredictionCalibrator
	liveScore         float64
}

/*
NewTrackStore creates an empty Hawkes track store.
*/
func NewTrackStore() *TrackStore {
	return &TrackStore{
		bySymbol: make(map[string]*SymbolTrack),
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
ApplyPredictionFeedback updates excitation calibration from one settled forecast.
The calibration scales Hawkes excitation parameters on the next warm-started fit.
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
FitBivariate warm-starts joint Hawkes MLE from the symbol's prior fit.
*/
func (trackStore *TrackStore) FitBivariate(
	symbol string,
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
) BivariateFit {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	prior := BivariateFit{}

	if track.hasFit {
		prior = applyExcitationCalibration(track.fit, track.calibrator.Scale())
	}

	context, ok := newFitContext(buyEvents, sellEvents, horizon)

	if ok {
		track.minFitEvents = context.MinFitEvents
	}

	fit := fitBivariateWithPrior(buyEvents, sellEvents, horizon, prior)

	if fit.MuBuy > 0 {
		track.fit = fit
		track.hasFit = true
		track.recordIntensityRatio(fit.BuyIntensity / fit.MuBuy)
	}

	return fit
}

/*
BaselineIntensityFence returns the symbol's own excitation ratio fence.
*/
func (trackStore *TrackStore) BaselineIntensityFence(symbol string) float64 {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track, ok := trackStore.bySymbol[symbol]

	if !ok || len(track.intensityRatios) == 0 {
		return 1
	}

	fence := confidenceFence(track.intensityRatios)

	if fence <= 0 {
		return 1
	}

	return fence
}

/*
RecordScore stores one raw Hawkes score and returns a unit-scale confidence in [0, 1].
*/
func (trackStore *TrackStore) RecordScore(symbol string, rawScore float64) float64 {
	if rawScore <= 0 {
		return 0
	}

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	track := trackStore.ensure(symbol)
	normalized := track.normalizedConfidence(rawScore)
	track.recordConfidence(rawScore)
	track.liveScore = normalized

	return normalized
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

func (track *SymbolTrack) normalizedConfidence(rawScore float64) float64 {
	if rawScore <= 0 {
		return 0
	}

	fence := confidenceFence(track.confidenceHistory)

	if fence <= 0 {
		return 1
	}

	if rawScore >= fence {
		return 1
	}

	return rawScore / fence
}

func (track *SymbolTrack) recordConfidence(confidence float64) {
	if confidence <= 0 {
		return
	}

	capacity := confidenceHistoryCap(track.minFitEvents)
	track.confidenceHistory = append(track.confidenceHistory, confidence)

	if len(track.confidenceHistory) > capacity {
		track.confidenceHistory = track.confidenceHistory[len(track.confidenceHistory)-capacity:]
	}
}

func (track *SymbolTrack) recordIntensityRatio(ratio float64) {
	if ratio <= 0 {
		return
	}

	capacity := confidenceHistoryCap(track.minFitEvents)
	track.intensityRatios = append(track.intensityRatios, ratio)

	if len(track.intensityRatios) > capacity {
		track.intensityRatios = track.intensityRatios[len(track.intensityRatios)-capacity:]
	}
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolTrack {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolTrack{
		confidenceHistory: make([]float64, 0, confidenceHistoryCap(bivariateParamCount*2)),
		intensityRatios:   make([]float64, 0, confidenceHistoryCap(bivariateParamCount*2)),
		minFitEvents:      bivariateParamCount * 2,
		calibrator:        engine.NewPredictionCalibrator(),
	}
	trackStore.bySymbol[symbol] = track

	return track
}
