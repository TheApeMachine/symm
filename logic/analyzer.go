package logic

import (
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

type Analyzer struct {
	thesis    *strategy.Thesis
	manifold  *Manifold
	resonance *Resonance
	causal    *Causal
}

func NewAnalyzer(thesis *strategy.Thesis) *Analyzer {
	return &Analyzer{
		thesis:    thesis,
		manifold:  NewManifold(thesis),
		resonance: NewResonance(thesis),
		causal:    NewCausal(thesis),
	}
}

func (analyzer *Analyzer) Close() {
	analyzer.manifold = nil
}

/*
Update turns measurements into particles that "surf" on the phase-directed pilot-wave
driven by the oscillator field underneath the compressed gas fluid.
*/
func (analyzer *Analyzer) Update(
	measurements []types.Measurement,
) *strategy.Thesis {
	analyzer.thesis = analyzer.manifold.Update(measurements)
	analyzer.thesis = analyzer.resonance.Update()
	analyzer.thesis = analyzer.causal.Update()
	return analyzer.thesis
}
