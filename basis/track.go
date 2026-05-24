package basis

import (
	"sync"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/stats"
)

const historyCap = 24

/*
SymbolTrack stores relative-strength history for one pair.
*/
type SymbolTrack struct {
	engine.SymbolLock
	relStrength    []float64
	calibrator     engine.PredictionCalibrator
	confidenceHist []float64
	liveScore      float64
}

/*
TrackStore holds cross-section basis state.
*/
type TrackStore struct {
	engine.GaugeScan
	mu       sync.RWMutex
	bySymbol map[string]*SymbolTrack
}

/*
NewTrackStore creates an empty basis track store.
*/
func NewTrackStore() *TrackStore {
	return &TrackStore{
		bySymbol: make(map[string]*SymbolTrack),
	}
}

func (trackStore *TrackStore) BeginScan() {
	trackStore.ResetGaugeScan()
}

func (trackStore *TrackStore) ApplyPredictionFeedback(feedback engine.PredictionFeedback) {
	if feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	track := trackStore.ensure(feedback.Symbol)

	track.Lock()
	defer track.Unlock()

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
		relStrength:    make([]float64, 0, historyCap),
		confidenceHist: make([]float64, 0, historyCap),
		calibrator:     engine.NewPredictionCalibrator(),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func (trackStore *TrackStore) recordScore(symbol string, rawScore float64) float64 {
	if rawScore <= 0 {
		return 0
	}

	track := trackStore.ensure(symbol)

	track.Lock()
	defer track.Unlock()

	normalized := engine.NormalizeConfidence(rawScore, track.confidenceHist)
	track.liveScore = normalized
	track.confidenceHist = append(track.confidenceHist, rawScore)

	if len(track.confidenceHist) > historyCap {
		track.confidenceHist = track.confidenceHist[len(track.confidenceHist)-historyCap:]
	}

	trackStore.ObserveGaugeScore(normalized)

	return normalized
}

func (track *SymbolTrack) recordRelativeStrength(value float64) {
	track.Lock()
	defer track.Unlock()

	track.relStrength = append(track.relStrength, value)

	if len(track.relStrength) > historyCap {
		track.relStrength = track.relStrength[len(track.relStrength)-historyCap:]
	}
}

func crossSectionMedianChange(changes map[string]float64) float64 {
	values := make([]float64, 0, len(changes))

	for _, change := range changes {
		values = append(values, change)
	}

	return stats.Median(values)
}

func basisScore(changePct, crossMedian float64) float64 {
	if !validChange(changePct) || !validChange(crossMedian) {
		return absFloat(changePct - crossMedian)
	}

	return absFloat(changePct - crossMedian)
}

func validChange(value float64) bool {
	return value != 0
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}

	return value
}
