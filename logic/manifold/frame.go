package manifold

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
projectFrame is one shared-field publish read: GPU display texture plus resident
omega spectrum. Phase scans stay per-symbol in phase.
*/
type projectFrame struct {
	grid    pfluid.Grid
	display []byte
	width   int
	height  int
	stats   pfluid.DisplayStats
	wave    []pfluid.WaveMode
}

/*
project reads the GPU display texture and resident omega spectrum for every
GasReady symbol view. Phase scans stay per-symbol in phase.
*/
func (solver *Solver) project() (projectFrame, error) {
	frame := projectFrame{grid: solver.config.Grid}

	if solver.domain == nil || solver.domain.ParticleCount() == 0 {
		return frame, nil
	}

	wave, err := solver.domain.Wave()

	if err != nil {
		return frame, errnie.Err(
			errnie.Internal,
			"manifold: failed to read shared omega spectrum",
			err,
		)
	}

	rgba, stats, err := solver.domain.Display()

	if err != nil {
		return frame, errnie.Err(
			errnie.Internal,
			"manifold: failed to read shared display texture",
			err,
		)
	}

	frame.display = rgba
	frame.width = int(stats.Width)
	frame.height = int(stats.Height)
	frame.stats = stats
	frame.wave = wave

	return frame, nil
}

/*
phase stages the current resident omega field until cognition commits a label
for the same symbol epoch, then scans one complete mode-derived turn against
already labeled market states for that symbol. wave is the shared spectrum
already read for this publish; it is not re-fetched per symbol.
*/
func (solver *Solver) phase(
	symbol string,
	at time.Time,
	advanced bool,
	wave []pfluid.WaveMode,
) ([]PhaseResponse, error) {
	if symbol == "" || len(wave) == 0 {
		return nil, nil
	}

	dial := make(geometry.PhaseDial, len(wave))
	hasAmplitude := false

	for index, mode := range wave {
		dial[index] = complex(float64(mode.Real), float64(mode.Imaginary))
		hasAmplitude = hasAmplitude || mode.Real != 0 || mode.Imaginary != 0
	}

	if !hasAmplitude {
		return nil, nil
	}

	if advanced {
		solver.Stage(symbol, at, dial)
	}

	phaseScan, err := solver.Responses(symbol, dial, at)

	if err != nil {
		return nil, err
	}

	return phaseScan, nil
}

/*
paint attaches the shared GPU display texture and wave reading to a state view
so the dashboard can blit any symbol without a backend focus gate.
*/
func (solver *Solver) paint(
	state *State,
	frame projectFrame,
	phaseScan []PhaseResponse,
	population int,
) {
	if state == nil || len(frame.display) == 0 {
		return
	}

	state.Grid = frame.grid
	state.Display = frame.display
	state.DisplayWidth = frame.width
	state.DisplayHeight = frame.height
	state.RhoOccupied = int(frame.stats.RhoOccupied)
	state.PsiOccupied = int(frame.stats.PsiOccupied)
	state.RhoMax = float64(frame.stats.RhoMax)
	state.PsiMax = float64(frame.stats.PsiMax)
	state.OscillatorCount = population
	state.SharedOscillatorCount = population
	state.Wave = frame.wave
	state.PhaseScan = phaseScan
	state.PhaseReady = len(frame.wave) > 0 && len(phaseScan) == len(frame.wave)
	state.PhaseReason = solver.phaseReason(state.PhaseReady, frame.wave)
}

/*
phaseReason explains why a wave cannot yet be scanned. A zero-amplitude wave
has no defined phase direction; a nonzero wave needs an earlier corpus
observation before its phase response has a meaningful comparison target.
*/
func (solver *Solver) phaseReason(
	ready bool,
	wave []pfluid.WaveMode,
) string {
	if ready {
		return ""
	}

	for _, mode := range wave {
		if mode.Real != 0 || mode.Imaginary != 0 {
			return "awaiting a prior outcome-labeled phase observation"
		}
	}

	return "resident wave amplitude is zero"
}
