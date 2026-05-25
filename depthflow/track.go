package depthflow

import (
	"sync"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/market"
)

const historyCap = 24

/*
SymbolTrack holds depth-imbalance history for one pair.
*/
type SymbolTrack struct {
	engine.ScoreRecorder
	depthImbalance []float64
}

/*
TrackStore holds per-symbol depth-flow state.
*/
type TrackStore struct {
	engine.GaugeScan
	mu                sync.RWMutex
	bySymbol          map[string]*SymbolTrack
	calibrationParams engine.CalibrationParams
}

/*
NewTrackStore creates an empty depth-flow track store.
*/
func NewTrackStore(calibrationParams engine.CalibrationParams) *TrackStore {
	return &TrackStore{
		bySymbol:          make(map[string]*SymbolTrack),
		calibrationParams: calibrationParams,
	}
}

/*
BeginScan resets live gauge accumulators for one measure pass.
*/
func (trackStore *TrackStore) BeginScan() {
	trackStore.ResetGaugeScan()
}

/*
ApplyPredictionFeedback updates depth calibration from one settled forecast.
*/
func (trackStore *TrackStore) ApplyPredictionFeedback(feedback engine.PredictionFeedback) {
	if feedback.Symbol == "" || feedback.PredictedReturn <= 0 {
		return
	}

	trackStore.mu.Lock()
	track := trackStore.ensureLocked(feedback.Symbol)
	trackStore.mu.Unlock()

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
		depthImbalance: make([]float64, 0, historyCap),
		ScoreRecorder:  engine.NewScoreRecorder(trackStore.calibrationParams, historyCap),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func (trackStore *TrackStore) recordCalibrated(symbol string, rawScore float64) float64 {
	track := trackStore.ensure(symbol)

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	return track.RecordCalibrated(rawScore, &trackStore.GaugeScan)
}

func (track *SymbolTrack) recordDepthImbalance(value float64) {
	if value == 0 {
		return
	}

	track.depthImbalance = append(track.depthImbalance, value)

	if len(track.depthImbalance) > historyCap {
		track.depthImbalance = track.depthImbalance[len(track.depthImbalance)-historyCap:]
	}
}

func depthImbalanceAtLevels(bids, asks []market.BookLevel) float64 {
	bidVol := levelVolume(bids)
	askVol := levelVolume(asks)
	total := bidVol + askVol

	if total <= 0 {
		return 0
	}

	return (bidVol - askVol) / total
}

func levelVolume(levels []market.BookLevel) float64 {
	total := 0.0

	for _, level := range levels {
		if level.Volume > 0 {
			total += level.Volume
		}
	}

	return total
}
