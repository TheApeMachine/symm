package manifold

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
project reads and converts the shared field projection and resident omega
spectrum once for all GasReady symbol views. Phase scans stay per-symbol.
*/
func (solver *Solver) project() (State, error) {
	if len(solver.particles) == 0 {
		return State{Grid: solver.config.Grid}, nil
	}

	wave, err := solver.domain.Wave()

	if err != nil {
		return State{}, errnie.Err(
			errnie.Internal,
			"manifold: failed to read shared omega spectrum",
			err,
		)
	}

	projection, err := solver.domain.Projection()

	if err != nil {
		return State{}, errnie.Err(
			errnie.Internal,
			"manifold: failed to read shared field projection",
			err,
		)
	}

	return State{
		Grid:         projection.Grid,
		Rho:          projectionRows(projection.Density, projection.Grid),
		PsiMag2:      projectionRows(projection.Coherence, projection.Grid),
		GuidanceVelX: projectionRows(projection.GuidanceX, projection.Grid),
		GuidanceVelZ: projectionRows(projection.GuidanceZ, projection.Grid),
		Wave:         wave,
	}, nil
}

/*
phase stages the current resident omega field until cognition commits a label
for the same symbol epoch, then scans one complete mode-derived turn against
already labeled market states for that symbol.
*/
func (solver *Solver) phase(
	symbol string,
	at time.Time,
	advanced bool,
	wave []pfluid.WaveMode,
) ([]PhaseResponse, error) {
	if symbol == "" {
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
paint attaches the shared field and one symbol's latest observations to a state
view so the dashboard can render any symbol without a backend focus gate.
*/
func (solver *Solver) paint(
	state *State,
	projection *State,
	phaseScan []PhaseResponse,
	slot *symbolSlot,
) {
	if state == nil || projection == nil || slot == nil || slot.start < 0 ||
		slot.end < slot.start || slot.end > len(solver.particles) ||
		len(projection.Rho) == 0 {
		return
	}

	state.Grid = projection.Grid
	state.Rho = projection.Rho
	state.PsiMag2 = projection.PsiMag2
	state.GuidanceVelX = projection.GuidanceVelX
	state.GuidanceVelZ = projection.GuidanceVelZ
	state.Particles = renderParticles(
		solver.particles[slot.start:slot.end],
		projection.Grid,
	)
	state.SharedOscillatorCount = len(solver.particles)
	state.Wave = projection.Wave
	state.PhaseScan = phaseScan
	state.PhaseReady = len(projection.Wave) > 0 &&
		len(phaseScan) == len(projection.Wave)
	state.PhaseReason = solver.phaseReason(state.PhaseReady, projection.Wave)
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
established cell-based dashboard payload.
*/
func renderParticles(
	particles []pfluid.Particle,
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
	}

	return rendered
}
