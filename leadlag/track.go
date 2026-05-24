package leadlag

import (
	"sync"

	"github.com/theapemachine/symm/engine"
)

const historyCap = 32

/*
SymbolTrack stores return history for lead-lag scoring.
*/
type SymbolTrack struct {
	returns        []float64
	calibrator     engine.PredictionCalibrator
	confidenceHist []float64
	lastPrice      float64
	hasLast        bool
	liveScore      float64
}

/*
TrackStore holds cross-symbol lead-lag state.
*/
type TrackStore struct {
	engine.GaugeScan
	mu       sync.RWMutex
	bySymbol map[string]*SymbolTrack
	leader   string
}

/*
NewTrackStore creates an empty lead-lag track store.
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
		returns:        make([]float64, 0, historyCap),
		confidenceHist: make([]float64, 0, historyCap),
		calibrator:     engine.NewPredictionCalibrator(),
	}
	trackStore.bySymbol[symbol] = track

	return track
}

func (trackStore *TrackStore) recordReturn(symbol string, last float64) float64 {
	if last <= 0 {
		return 0
	}

	track := trackStore.ensure(symbol)

	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	if !track.hasLast || track.lastPrice <= 0 {
		track.lastPrice = last
		track.hasLast = true

		return 0
	}

	ret := last/track.lastPrice - 1
	track.lastPrice = last
	track.returns = append(track.returns, ret)

	if len(track.returns) > historyCap {
		track.returns = track.returns[len(track.returns)-historyCap:]
	}

	return ret
}

func (trackStore *TrackStore) setLeader(symbol string) {
	trackStore.mu.Lock()
	defer trackStore.mu.Unlock()

	trackStore.leader = symbol
}

/*
Leader returns the current cross-section volume leader symbol.
*/
func (trackStore *TrackStore) Leader() string {
	trackStore.mu.RLock()
	defer trackStore.mu.RUnlock()

	return trackStore.leader
}

func (trackStore *TrackStore) leaderReturn() float64 {
	trackStore.mu.RLock()
	defer trackStore.mu.RUnlock()

	if trackStore.leader == "" {
		return 0
	}

	track, ok := trackStore.bySymbol[trackStore.leader]

	if !ok || len(track.returns) == 0 {
		return 0
	}

	return track.returns[len(track.returns)-1]
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

func leadLagScore(leaderReturn, followerReturn float64) float64 {
	if leaderReturn == 0 {
		return 0
	}

	lag := leaderReturn - followerReturn

	if leaderReturn > 0 && lag > 0 {
		return lag
	}

	if leaderReturn < 0 && lag < 0 {
		return -lag
	}

	return 0
}

func pickLeader(snapshots map[string]float64) string {
	bestSymbol := ""
	bestVolume := 0.0

	for symbol, volume := range snapshots {
		if volume > bestVolume {
			bestVolume = volume
			bestSymbol = symbol
		}
	}

	return bestSymbol
}
