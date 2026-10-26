package tokenizer

type mdlCandidateState struct {
	K      int     `json:"k"`
	Gain   float64 `json:"gain"`
	Forced bool    `json:"forced"`
}

type distributionState struct {
	Counts   [256]int `json:"counts"`
	Total    int      `json:"total"`
	Distinct int      `json:"distinct"`
}

// MDLState is the segmenter memory a restart has to load before the next
// letter, or the following boundary would fall in a different place.
type MDLState struct {
	Buf          []byte              `json:"buf"`
	Offset       int                 `json:"offset"`
	Dist         distributionState   `json:"dist"`
	Running      distributionState   `json:"running"`
	Candidates   []mdlCandidateState `json:"candidates"`
	PrevSegLen   int                 `json:"prev_seg_len"`
	FluxEmitted  bool                `json:"flux_emitted"`
	LastByteVal  float64             `json:"last_byte_val"`
	LastEigenMag float64             `json:"last_eigen_mag"`
	EmaPhase     float64             `json:"ema_phase"`
	EmaPop       float64             `json:"ema_pop"`
	RunningMeta  uint64              `json:"running_meta"`
	LastGain     float64             `json:"last_gain"`
	EigenTrained bool                `json:"eigen_trained"`
	Calibrator   calibratorState     `json:"calibrator"`
}

func (seq *MDL) State() MDLState {
	if seq == nil {
		return MDLState{}
	}

	out := MDLState{
		Buf:          append([]byte(nil), seq.buf...),
		Offset:       seq.offset,
		Dist:         distributionSnapshot(seq.dist),
		Running:      distributionSnapshot(seq.runningHist),
		PrevSegLen:   seq.prevSegLen,
		FluxEmitted:  seq.fluxEmitted,
		LastByteVal:  seq.lastByteVal,
		LastEigenMag: seq.lastEigenMag,
		EmaPhase:     seq.emaPhase,
		EmaPop:       seq.emaPop,
		RunningMeta:  uint64(seq.runningMeta),
		LastGain:     seq.lastGain,
		Candidates:   make([]mdlCandidateState, 0, len(seq.candidates)),
	}
	if seq.eigen != nil {
		out.EigenTrained = seq.eigen.Trained
	}
	if seq.calibrator != nil {
		out.Calibrator = seq.calibrator.state()
	}
	for _, item := range seq.candidates {
		out.Candidates = append(out.Candidates, mdlCandidateState{
			K:      item.k,
			Gain:   item.gain,
			Forced: item.forced,
		})
	}
	return out
}

func (seq *MDL) Restore(state MDLState) {
	if seq == nil {
		return
	}

	seq.buf = append([]byte(nil), state.Buf...)
	seq.offset = state.Offset
	seq.dist = distributionFrom(state.Dist)
	seq.runningHist = distributionFrom(state.Running)
	seq.prevSegLen = state.PrevSegLen
	seq.fluxEmitted = state.FluxEmitted
	seq.lastByteVal = state.LastByteVal
	seq.lastEigenMag = state.LastEigenMag
	seq.emaPhase = state.EmaPhase
	seq.emaPop = state.EmaPop
	seq.runningMeta = SegmentMeta(state.RunningMeta)
	seq.lastGain = state.LastGain
	seq.candidates = make([]candidate, 0, len(state.Candidates))
	for _, item := range state.Candidates {
		seq.candidates = append(seq.candidates, candidate{
			k:      item.K,
			gain:   item.Gain,
			forced: item.Forced,
		})
	}
	if seq.eigen == nil {
		seq.eigen = NewEigenMode()
	}
	seq.eigen.Trained = state.EigenTrained
	if seq.calibrator == nil {
		seq.calibrator = NewCalibrator()
	}
	seq.calibrator.restore(state.Calibrator)
}

func distributionSnapshot(dist *Distribution) distributionState {
	if dist == nil {
		return distributionState{}
	}
	return distributionState{
		Counts:   dist.counts,
		Total:    dist.total,
		Distinct: dist.distinct,
	}
}

func distributionFrom(state distributionState) *Distribution {
	dist := NewDistribution()
	dist.counts = state.Counts
	dist.total = state.Total
	dist.distinct = state.Distinct
	return dist
}
