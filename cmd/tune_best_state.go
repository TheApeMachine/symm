package cmd

import "sync"

type tuneBestState struct {
	mu            sync.Mutex
	hasBest       bool
	selection     float64
	trainScore    float64
	holdoutScores []float64
	gap           float64
	config        tuneCandidate
}

type tuneBestSnapshot struct {
	hasBest       bool
	selection     float64
	trainScore    float64
	holdoutScores []float64
	gap           float64
	config        tuneCandidate
}

func newTuneBestState() *tuneBestState {
	return &tuneBestState{}
}

func (state *tuneBestState) Snapshot() tuneBestSnapshot {
	state.mu.Lock()
	defer state.mu.Unlock()

	return tuneBestSnapshot{
		hasBest:       state.hasBest,
		selection:     state.selection,
		trainScore:    state.trainScore,
		holdoutScores: append([]float64(nil), state.holdoutScores...),
		gap:           state.gap,
		config:        state.config,
	}
}

func (state *tuneBestState) UpdateIfBetter(
	candidate tuneCandidate,
	scores trialScores,
) (bool, tuneCandidate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.hasBest && !betterTuneCandidate(
		scores.selection,
		scores.trainScore,
		state.selection,
		state.trainScore,
	) {
		return false, tuneCandidate{}
	}

	state.hasBest = true
	state.selection = scores.selection
	state.trainScore = scores.trainScore
	state.holdoutScores = append([]float64(nil), scores.holdoutScores...)
	state.gap = scores.gap
	state.config = snapshotTuneCandidate(*candidate.perspectives, candidate.tunables)

	return true, state.config
}

func (state *tuneBestState) SetBaseline(
	candidate tuneCandidate,
	scores trialScores,
) tuneCandidate {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.hasBest = true
	state.selection = scores.selection
	state.trainScore = scores.trainScore
	state.holdoutScores = append([]float64(nil), scores.holdoutScores...)
	state.gap = scores.gap
	state.config = snapshotTuneCandidate(*candidate.perspectives, candidate.tunables)

	return state.config
}
