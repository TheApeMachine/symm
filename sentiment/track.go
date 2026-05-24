package sentiment

import (
	"math"
	"sync"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/stats"
)

const historyCap = 24

/*
SymbolTrack stores sentiment feature history for one pair.
*/
type SymbolTrack struct {
	sentiment      []float64
	calibrator     engine.PredictionCalibrator
	confidenceHist []float64
	liveScore      float64
}

/*
TrackStore holds cross-section sentiment state.
*/
type TrackStore struct {
	engine.GaugeScan
	mu                sync.RWMutex
	bySymbol          map[string]*SymbolTrack
	calibrationParams engine.CalibrationParams
}

/*
NewTrackStore creates an empty sentiment track store.
*/
func NewTrackStore(calibrationParams engine.CalibrationParams) *TrackStore {
	return &TrackStore{
		bySymbol:          make(map[string]*SymbolTrack),
		calibrationParams: calibrationParams,
	}
}

func (trackStore *TrackStore) BeginScan() {
	trackStore.ResetGaugeScan()
}

func (trackStore *TrackStore) ApplyPredictionFeedback(feedback engine.PredictionFeedback) {
	if feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	trackStore.mu.Lock()
	track := trackStore.ensureLocked(feedback.Symbol)
	trackStore.mu.Unlock()

	track.calibrator.Apply(feedback)
}

func (trackStore *TrackStore) ensure(symbol string) *SymbolTrack {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	return trackStore.ensureLocked(symbol)
}

func (trackStore *TrackStore) ensureLocked(symbol string) *SymbolTrack {
	track, ok := trackStore.bySymbol[symbol]

	if ok {
		return track
	}

	track = &SymbolTrack{
		sentiment:      make([]float64, 0, historyCap),
		confidenceHist: make([]float64, 0, historyCap),
		calibrator:     engine.NewPredictionCalibrator(trackStore.calibrationParams),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func (trackStore *TrackStore) recordScore(symbol string, rawScore float64) float64 {
	if rawScore <= 0 {
		return 0
	}

	track := trackStore.ensure(symbol)

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	normalized := track.calibrator.NormalizeConfidence(rawScore, track.confidenceHist)
	track.liveScore = normalized
	track.confidenceHist = append(track.confidenceHist, rawScore)

	if len(track.confidenceHist) > historyCap {
		track.confidenceHist = track.confidenceHist[len(track.confidenceHist)-historyCap:]
	}

	trackStore.ObserveGaugeScore(normalized)

	return normalized
}

func (track *SymbolTrack) recordSentiment(value float64) {
	track.sentiment = append(track.sentiment, value)

	if len(track.sentiment) > historyCap {
		track.sentiment = track.sentiment[len(track.sentiment)-historyCap:]
	}
}

func crossSectionZScore(value float64, values []float64) float64 {
	if len(values) < 2 {
		return 0
	}

	mean := stats.Mean(values)
	sorted := stats.CopySorted(values)
	median := stats.PercentileSorted(sorted, 0.5)
	spread := stats.MedianAbsoluteDeviation(sorted, median)

	if spread <= 0 {
		return 0
	}

	return (value - mean) / spread
}

func sentimentRaw(pressureZ, changeZ float64) float64 {
	combined := 0.6*pressureZ + 0.4*changeZ

	if combined <= 0 {
		return 0
	}

	return math.Min(combined, 3)
}
