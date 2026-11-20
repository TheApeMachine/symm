package tokenizer

import "maps"

/*
symbolHistogramState is the JSON form of dist for MDLState persistence.
*/
type symbolHistogramState struct {
	Counts   map[uint32]int `json:"counts"`
	Total    int            `json:"total"`
	Distinct int            `json:"distinct"`
}

/*
MDLState is serializable segmenter memory — Restore must rebuild dist before the
next symbol or boundary search starts from an empty histogram.
*/
type MDLState struct {
	Stream   []uint32             `json:"stream"`
	Offset   int                  `json:"offset"`
	Vocab    map[string]uint32    `json:"vocab"`
	NextID   uint32               `json:"next_id"`
	Dist     symbolHistogramState `json:"dist"`
	LastGain float64              `json:"last_gain"`
}

/*
State snapshots stream, vocab, and dist for GrammarCatalog Persist after each MDL cut.
*/
func (mdl *MDL) State() MDLState {
	return MDLState{
		Stream:   append([]uint32(nil), mdl.stream...),
		Offset:   mdl.offset,
		Vocab:    cloneVocab(mdl.vocab),
		NextID:   mdl.nextID,
		Dist:     histogramSnapshot(mdl.dist),
		LastGain: mdl.lastGain,
	}
}

/*
Restore replays a persisted segmenter without re-running Analyze history.
*/
func (mdl *MDL) Restore(state MDLState) {
	mdl.stream = append([]uint32(nil), state.Stream...)
	mdl.offset = state.Offset
	mdl.vocab = cloneVocab(state.Vocab)
	mdl.nextID = state.NextID

	if mdl.nextID == 0 {
		mdl.nextID = 1
	}

	if mdl.vocab == nil {
		mdl.vocab = make(map[string]uint32)
	}

	mdl.dist = histogramFrom(state.Dist)
	mdl.lastGain = state.LastGain
}

/*
cloneVocab deep-copies vocab on export so persisted state does not alias live maps.
*/
func cloneVocab(vocab map[string]uint32) map[string]uint32 {
	if len(vocab) == 0 {
		return make(map[string]uint32)
	}

	out := make(map[string]uint32, len(vocab))
	maps.Copy(out, vocab)

	return out
}

/*
histogramSnapshot serializes dist without exposing private histogram fields to JSON tags elsewhere.
*/
func histogramSnapshot(hist *symbolHistogram) symbolHistogramState {
	if hist == nil {
		return symbolHistogramState{Counts: make(map[uint32]int)}
	}

	counts := make(map[uint32]int, len(hist.counts))
	for id, n := range hist.counts {
		counts[id] = n
	}

	return symbolHistogramState{
		Counts:   counts,
		Total:    hist.total,
		Distinct: hist.numDistinct,
	}
}

/*
histogramFrom rebuilds dist after Restore for continued boundary search.
*/
func histogramFrom(state symbolHistogramState) *symbolHistogram {
	hist := newSymbolHistogram()
	hist.total = state.Total
	hist.numDistinct = state.Distinct

	for id, n := range state.Counts {
		hist.counts[id] = n
	}

	return hist
}
