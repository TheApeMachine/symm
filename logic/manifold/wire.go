package manifold

import (
	"time"

	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
WireGrid is the X–Z display extent the fluid canvas needs (Y is collapsed).
*/
type WireGrid struct {
	X       uint32  `json:"x"`
	Z       uint32  `json:"z"`
	Spacing float32 `json:"spacing"`
}

/*
WireReading is the scalar panel surface painted by xray / meta.
*/
type WireReading struct {
	PressureGradX  float64 `json:"pressureGradX"`
	PressureGradZ  float64 `json:"pressureGradZ"`
	Divergence     float64 `json:"divergence"`
	CoherenceMag2  float64 `json:"coherenceMag2"`
	GuidanceSpeed  float64 `json:"guidanceSpeed"`
	ViscosityProxy float64 `json:"viscosityProxy"`
}

/*
WireField is the lightweight manifold meta packet. Dense lattices travel as
separate binary frames so JSON never carries 64×64 decimal grids.
*/
type WireField struct {
	Source                string          `json:"source"`
	Symbol                string          `json:"symbol"`
	At                    time.Time       `json:"at"`
	OscillatorCount       int             `json:"oscillatorCount"`
	SharedOscillatorCount int             `json:"sharedOscillatorCount"`
	Grid                  WireGrid        `json:"grid"`
	Reading               WireReading     `json:"reading"`
	PhaseReady            bool            `json:"phaseReady"`
	PhaseReason           string          `json:"phaseReason,omitempty"`
	PhaseScan             []PhaseResponse `json:"phaseScan,omitempty"`
}

/*
WireParticle is the oscillator cloud the fluid canvas aggregates — only fields
the painter reads.
*/
type WireParticle struct {
	CellX     float64 `json:"cell_x"`
	CellZ     float64 `json:"cell_z"`
	Phase     float64 `json:"phase"`
	Amplitude float64 `json:"amplitude"`
	VelX      float64 `json:"vel_x"`
	VelZ      float64 `json:"vel_z"`
}

/*
WireParticles is the particle packet for one shared-field publish.
*/
type WireParticles struct {
	Source    string         `json:"source"`
	Symbol    string         `json:"symbol"`
	At        time.Time      `json:"at"`
	Particles []WireParticle `json:"particles,omitempty"`
}

/*
WireMode is one complex omega mode the phase dial needs.
*/
type WireMode struct {
	Omega     float32 `json:"omega"`
	Real      float32 `json:"real"`
	Imaginary float32 `json:"imaginary"`
	Linewidth float32 `json:"linewidth"`
}

/*
WireWave is the wave-mode packet for one shared-field publish.
*/
type WireWave struct {
	Source string     `json:"source"`
	Symbol string     `json:"symbol"`
	At     time.Time  `json:"at"`
	Wave   []WireMode `json:"wave,omitempty"`
}

/*
WirePackets splits one State into meta JSON, uint16 lattice binaries, particles,
and wave so each part can fan out under hub backpressure.
*/
func (state State) WirePackets() (WireField, [][]byte, WireParticles, WireWave) {
	field := WireField{
		Source:                state.Source,
		Symbol:                state.Symbol,
		At:                    state.At,
		OscillatorCount:       state.OscillatorCount,
		SharedOscillatorCount: state.SharedOscillatorCount,
		Grid: WireGrid{
			X:       uint32(state.Grid.X),
			Z:       uint32(state.Grid.Z),
			Spacing: state.Grid.Spacing,
		},
		Reading: WireReading{
			PressureGradX:  state.Reading.PressureGradX,
			PressureGradZ:  state.Reading.PressureGradZ,
			Divergence:     state.Reading.Divergence,
			CoherenceMag2:  state.Reading.CoherenceMag2,
			GuidanceSpeed:  state.Reading.GuidanceSpeed,
			ViscosityProxy: state.Reading.ViscosityProxy,
		},
		PhaseReady:  state.PhaseReady,
		PhaseReason: state.PhaseReason,
		PhaseScan:   state.PhaseScan,
	}

	lattices := make([][]byte, 0, 4)

	for _, plane := range []struct {
		kind uint8
		grid [][]float64
	}{
		{BinaryKindRho, state.Rho},
		{BinaryKindPsi, state.PsiMag2},
		{BinaryKindGuidanceX, state.GuidanceVelX},
		{BinaryKindGuidanceZ, state.GuidanceVelZ},
	} {
		encoded, ok := EncodeLattice(plane.kind, state.Symbol, state.At, plane.grid)

		if !ok {
			continue
		}

		lattices = append(lattices, encoded)
	}

	particles := WireParticles{
		Source:    state.Source,
		Symbol:    state.Symbol,
		At:        state.At,
		Particles: make([]WireParticle, 0, len(state.Particles)),
	}

	for _, particle := range state.Particles {
		particles.Particles = append(particles.Particles, WireParticle{
			CellX:     particle.CellX,
			CellZ:     particle.CellZ,
			Phase:     particle.Phase,
			Amplitude: particle.Amplitude,
			VelX:      particle.VelX,
			VelZ:      particle.VelZ,
		})
	}

	wave := WireWave{
		Source: state.Source,
		Symbol: state.Symbol,
		At:     state.At,
		Wave:   make([]WireMode, 0, len(state.Wave)),
	}

	for _, mode := range state.Wave {
		wave.Wave = append(wave.Wave, wireMode(mode))
	}

	return field, lattices, particles, wave
}

func wireMode(mode pfluid.WaveMode) WireMode {
	return WireMode{
		Omega:     mode.Omega,
		Real:      mode.Real,
		Imaginary: mode.Imaginary,
		Linewidth: mode.Linewidth,
	}
}
