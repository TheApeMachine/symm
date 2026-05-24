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
	depthImbalance []float64
	calibrator     engine.PredictionCalibrator
	confidenceHist []float64
	liveScore      float64
}

/*
TrackStore holds per-symbol depth-flow state.
*/
type TrackStore struct {
	engine.GaugeScan
	mu       sync.RWMutex
	bySymbol map[string]*SymbolTrack
}

/*
NewTrackStore creates an empty depth-flow track store.
*/
func NewTrackStore() *TrackStore {
	return &TrackStore{
		bySymbol: make(map[string]*SymbolTrack),
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
		depthImbalance: make([]float64, 0, historyCap),
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
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	normalized := engine.NormalizeConfidence(rawScore, track.confidenceHist)
	track.liveScore = normalized
	track.confidenceHist = append(track.confidenceHist, rawScore)

	if len(track.confidenceHist) > historyCap {
		track.confidenceHist = track.confidenceHist[len(track.confidenceHist)-historyCap:]
	}

	trackStore.ObserveGaugeScore(normalized)

	return normalized
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
