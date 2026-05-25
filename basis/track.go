package basis

import (
	"math"
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
	engine.ScoreRecorder
	relStrength []float64
}

/*
TrackStore holds cross-section basis state.
*/
type TrackStore struct {
	engine.GaugeScan
	mu                sync.RWMutex
	bySymbol          map[string]*SymbolTrack
	calibrationParams engine.CalibrationParams
}

/*
NewTrackStore creates an empty basis track store.
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
	if !engine.ValidPredictionFeedback(feedback) {
		return
	}

	track := trackStore.ensure(feedback.Symbol)

	track.Lock()
	defer track.Unlock()

	track.Calibrator.Apply(feedback)
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
		relStrength:   make([]float64, 0, historyCap),
		ScoreRecorder: engine.NewScoreRecorder(trackStore.calibrationParams, historyCap),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func (trackStore *TrackStore) recordCalibrated(symbol string, rawScore float64) float64 {
	track := trackStore.ensure(symbol)

	track.Lock()
	defer track.Unlock()

	return track.RecordCalibrated(rawScore, &trackStore.GaugeScan)
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
		return math.Abs(changePct - crossMedian)
	}

	return math.Abs(changePct - crossMedian)
}

func validChange(value float64) bool {
	return value != 0
}
