package tokenizer

import "maps"

type symbolHistogramState struct {
	Counts   map[uint32]int `json:"counts"`
	Total    int            `json:"total"`
	Distinct int            `json:"distinct"`
}

// MDLState is segmenter memory restored before the next grammar symbol.
type MDLState struct {
	Stream   []uint32             `json:"stream"`
	Offset   int                  `json:"offset"`
	Vocab    map[string]uint32    `json:"vocab"`
	NextID   uint32               `json:"next_id"`
	Dist     symbolHistogramState `json:"dist"`
	LastGain float64              `json:"last_gain"`
}

func (mdl *MDL) State() MDLState {
	if mdl == nil {
		return MDLState{}
	}

	return MDLState{
		Stream:   append([]uint32(nil), mdl.stream...),
		Offset:   mdl.offset,
		Vocab:    cloneVocab(mdl.vocab),
		NextID:   mdl.nextID,
		Dist:     histogramSnapshot(mdl.dist),
		LastGain: mdl.lastGain,
	}
}

func (mdl *MDL) Restore(state MDLState) {
	if mdl == nil {
		return
	}

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

	if mdl.dist == nil {
		mdl.dist = newSymbolHistogram()
	}
}

func cloneVocab(vocab map[string]uint32) map[string]uint32 {
	if len(vocab) == 0 {
		return make(map[string]uint32)
	}

	out := make(map[string]uint32, len(vocab))
	maps.Copy(out, vocab)

	return out
}

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

func histogramFrom(state symbolHistogramState) *symbolHistogram {
	hist := newSymbolHistogram()
	hist.total = state.Total
	hist.numDistinct = state.Distinct

	for id, n := range state.Counts {
		hist.counts[id] = n
	}

	return hist
}
