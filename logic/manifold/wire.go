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
WireField is the lightweight manifold meta packet. The field picture travels as
one GPU-composited RGBA texture; meta only carries scalars the panels need.
*/
type WireField struct {
	Source                string          `json:"source"`
	Symbol                string          `json:"symbol"`
	At                    time.Time       `json:"at"`
	OscillatorCount       int             `json:"oscillatorCount"`
	SharedOscillatorCount int             `json:"sharedOscillatorCount"`
	RhoOccupied           int             `json:"rhoOccupied"`
	PsiOccupied           int             `json:"psiOccupied"`
	RhoMax                float64         `json:"rhoMax"`
	PsiMax                float64         `json:"psiMax"`
	Grid                  WireGrid        `json:"grid"`
	Reading               WireReading     `json:"reading"`
	PhaseReady            bool            `json:"phaseReady"`
	PhaseReason           string          `json:"phaseReason,omitempty"`
	PhaseScan             []PhaseResponse `json:"phaseScan,omitempty"`
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
WirePackets splits one State into meta JSON, one GPU display texture, and wave
so the socket carries a blit-ready picture instead of raw planes.
*/
func (state State) WirePackets() (WireField, [][]byte, WireWave) {
	field := WireField{
		Source:                state.Source,
		Symbol:                state.Symbol,
		At:                    state.At,
		OscillatorCount:       state.OscillatorCount,
		SharedOscillatorCount: state.SharedOscillatorCount,
		RhoOccupied:           state.RhoOccupied,
		PsiOccupied:           state.PsiOccupied,
		RhoMax:                state.RhoMax,
		PsiMax:                state.PsiMax,
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

	if field.Grid.X == 0 && state.DisplayWidth > 0 {
		field.Grid.X = uint32(state.DisplayWidth)
	}

	if field.Grid.Z == 0 && state.DisplayHeight > 0 {
		field.Grid.Z = uint32(state.DisplayHeight)
	}

	lattices := make([][]byte, 0, 1)

	if encoded, ok := EncodeDisplay(
		state.Symbol, state.At, state.DisplayWidth, state.DisplayHeight, state.Display,
	); ok {
		lattices = append(lattices, encoded)
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

	return field, lattices, wave
}

func wireMode(mode pfluid.WaveMode) WireMode {
	return WireMode{
		Omega:     mode.Omega,
		Real:      mode.Real,
		Imaginary: mode.Imaginary,
		Linewidth: mode.Linewidth,
	}
}
