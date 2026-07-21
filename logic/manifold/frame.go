package manifold

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/geometry"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
)

/*
project records the resident spectrum after advances and reads the larger X-Z
projection only when the browser requests a focus.
*/
func (solver *Solver) project(
	focus string,
	at time.Time,
	advanced bool,
) (pfluid.Projection, []pfluid.WaveMode, []PhaseResponse, error) {
	if len(solver.particles) == 0 {
		return pfluid.Projection{Grid: solver.config.Grid}, nil, nil, nil
	}

	wave, scan, err := solver.phase(at, advanced, focus != "")

	if err != nil {
		return pfluid.Projection{}, nil, nil, err
	}

	if focus == "" {
		return pfluid.Projection{Grid: solver.config.Grid}, nil, nil, nil
	}

	projection, err := solver.domain.Projection()

	if err != nil {
		return pfluid.Projection{}, nil, nil, errnie.Err(
			errnie.Internal,
			"manifold: failed to read shared field projection",
			err,
		)
	}

	return projection, wave, scan, nil
}

/*
phase records the current resident omega field as a PhaseDial and, when a
focused view requests it, scans one complete mode-derived turn against prior
market states while excluding the current observation from its own results.
*/
func (solver *Solver) phase(
	at time.Time,
	advanced bool,
	scan bool,
) ([]pfluid.WaveMode, []PhaseResponse, error) {
	if !advanced && !scan {
		return nil, nil, nil
	}

	wave, err := solver.domain.Wave()

	if err != nil {
		return nil, nil, errnie.Err(
			errnie.Internal,
			"manifold: failed to read shared omega spectrum",
			err,
		)
	}

	dial := make(geometry.PhaseDial, len(wave))
	hasAmplitude := false

	for index, mode := range wave {
		dial[index] = complex(float64(mode.Real), float64(mode.Imaginary))
		hasAmplitude = hasAmplitude || mode.Real != 0 || mode.Imaginary != 0
	}

	if !hasAmplitude {
		return wave, nil, nil
	}

	if advanced {
		if err := solver.spectrum.Insert(geometry.CorpusEntry[struct{}]{
			Dial: dial,
			At:   at,
		}); err != nil {
			return nil, nil, errnie.Err(
				errnie.UnprocessableContent,
				"manifold: failed to retain resident phase dial",
				err,
			)
		}
	}

	if !scan || solver.spectrum.Size() < 2 {
		return wave, nil, nil
	}

	phaseScan, err := solver.scan(dial, at)

	if err != nil {
		return nil, nil, err
	}

	return wave, phaseScan, nil
}

/*
scan samples one complete turn at the resident omega resolution and returns the
strongest prior response at each angle without manufacturing an outcome label.
*/
func (solver *Solver) scan(
	dial geometry.PhaseDial,
	at time.Time,
) ([]PhaseResponse, error) {
	modeCount := len(dial)
	angles := make([]float64, modeCount)

	for index := range angles {
		angles[index] = 2 * math.Pi * float64(index) / float64(modeCount)
	}

	responses, err := solver.spectrum.ScanPhasesExcluding(dial, angles, 1, at)

	if err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"manifold: failed to scan resident phase dial",
			err,
		)
	}

	phaseScan := make([]PhaseResponse, 0, len(responses))

	for index, matches := range responses {
		if len(matches) == 0 {
			continue
		}

		phaseScan = append(phaseScan, PhaseResponse{
			Angle:      angles[index],
			Similarity: matches[0].Similarity,
			ObservedAt: matches[0].At,
		})
	}

	return phaseScan, nil
}

/*
paint attaches the shared field and the focused symbol's latest observations to
one state without duplicating the full universe into every Thesis entry.
*/
func (solver *Solver) paint(
	state *State,
	projection pfluid.Projection,
	wave []pfluid.WaveMode,
	phaseScan []PhaseResponse,
	slot *symbolSlot,
) {
	if state == nil || slot == nil || slot.start < 0 || slot.end < slot.start ||
		slot.end > len(solver.particles) || len(projection.Density) == 0 {
		return
	}

	state.Grid = projection.Grid
	state.Rho = projectionRows(projection.Density, projection.Grid)
	state.PsiMag2 = projectionRows(projection.Coherence, projection.Grid)
	state.GuidanceVelX = projectionRows(projection.GuidanceX, projection.Grid)
	state.GuidanceVelZ = projectionRows(projection.GuidanceZ, projection.Grid)
	state.Particles = renderParticles(
		solver.particles[slot.start:slot.end],
		projection.Grid,
	)
	state.SharedOscillatorCount = len(solver.particles)
	state.Wave = wave
	state.PhaseScan = phaseScan
	state.PhaseReady = len(wave) > 0 && len(phaseScan) == len(wave)
	state.PhaseReason = solver.phaseReason(state.PhaseReady, wave)
}

/*
phaseReason explains why a focused wave cannot yet be scanned. A zero-amplitude
wave has no defined phase direction; a nonzero wave needs an earlier corpus
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
			return "awaiting a prior nonzero phase observation"
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
renderParticles converts the focused symbol's latest physical observations to
the established cell-based dashboard payload.
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
