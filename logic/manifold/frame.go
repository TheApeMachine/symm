package manifold

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
project reads the shared field projection and resident omega spectrum for every
GasReady symbol view. Phase scans stay per-symbol in phase.
*/
func (solver *Solver) project() (pfluid.Projection, []pfluid.WaveMode, error) {
	if solver.domain == nil || solver.domain.ParticleCount() == 0 {
		return pfluid.Projection{Grid: solver.config.Grid}, nil, nil
	}

	wave, err := solver.domain.Wave()

	if err != nil {
		return pfluid.Projection{}, nil, errnie.Err(
			errnie.Internal,
			"manifold: failed to read shared omega spectrum",
			err,
		)
	}

	projection, err := solver.domain.Projection()

	if err != nil {
		return pfluid.Projection{}, nil, errnie.Err(
			errnie.Internal,
			"manifold: failed to read shared field projection",
			err,
		)
	}

	return projection, wave, nil
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
paint attaches the shared field and one shared particle render to a state view
so the dashboard can render any symbol without a backend focus gate.
*/
func (solver *Solver) paint(
	state *State,
	grid pfluid.Grid,
	wave []pfluid.WaveMode,
	phaseScan []PhaseResponse,
	particles []Particle,
	rho, psi, guideX, guideZ [][]float64,
) {
	if state == nil || len(rho) == 0 {
		return
	}

	state.Grid = grid
	state.Rho = rho
	state.PsiMag2 = psi
	state.GuidanceVelX = guideX
	state.GuidanceVelZ = guideZ
	state.Particles = particles
	state.OscillatorCount = len(particles)
	state.SharedOscillatorCount = len(particles)
	state.Wave = wave
	state.PhaseScan = phaseScan
	state.PhaseReady = len(wave) > 0 && len(phaseScan) == len(wave)
	state.PhaseReason = solver.phaseReason(state.PhaseReady, wave)
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

/*
projectionRows converts the Metal row-major X-Z projection into dashboard rows
without changing its values or applying display normalization.
*/
func projectionRows(values []float32, grid pfluid.Grid) [][]float64 {
	if len(values) != grid.X*grid.Z {
		return nil
	}

	rows := make([][]float64, grid.Z)

	for cellZ := range grid.Z {
		rows[cellZ] = make([]float64, grid.X)

		for cellX := range grid.X {
			rows[cellZ][cellX] = float64(values[cellX+cellZ*grid.X])
		}
	}

	return rows
}

/*
renderParticles converts one symbol's latest physical observations to the
established cell-based dashboard payload, including post-merge spatial token IDs.
*/
func renderParticles(
	particles []pfluid.Particle,
	spatial []uint32,
	grid pfluid.Grid,
) []Particle {
	rendered := make([]Particle, len(particles))

	for index, particle := range particles {
		velocityX := float64(particle.Velocity.X)
		velocityY := float64(particle.Velocity.Y)
		velocityZ := float64(particle.Velocity.Z)
		rendered[index] = Particle{
			Role:      "particle",
			CellX:     float64(particle.Position.X / grid.Spacing),
			CellY:     float64(particle.Position.Y / grid.Spacing),
			CellZ:     float64(particle.Position.Z / grid.Spacing),
			Phase:     float64(particle.Phase),
			Omega:     float64(particle.Omega),
			Amplitude: math.Sqrt(float64(particle.Energy)),
			Heat:      float64(particle.Heat),
			VelX:      velocityX,
			VelY:      velocityY,
			VelZ:      velocityZ,
			Speed: math.Sqrt(
				velocityX*velocityX + velocityY*velocityY + velocityZ*velocityZ,
			),
		}

		if index < len(spatial) {
			rendered[index].SpatialTokenID = spatial[index]
		}
	}

	return rendered
}
