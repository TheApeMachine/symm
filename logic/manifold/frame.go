package manifold

import (
	"fmt"
	"math"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

/*
fieldProjection is the GPU price-time slice the terminal fluid canvas paints.
*/
type fieldProjection struct {
	Rho          [][]float64
	PsiMag2      [][]float64
	GuidanceVelX [][]float64
	GuidanceVelZ [][]float64
	Particles    []pmanifold.Particle
	Grid         pmanifold.Grid
}

/*
projectField reads the post-step gas and pilot-wave projections plus oscillator
carriers from the active GPU solver slot.
*/
func projectField(
	handle *pmanifold.Solver,
	config pmanifold.Config,
	oscillatorCount int,
) (fieldProjection, error) {
	projection := fieldProjection{
		Grid: pmanifold.Grid{
			X: config.GridX,
			Y: config.GridY,
			Z: config.GridZ,
		},
	}

	if handle == nil {
		return projection, nil
	}

	rho, err := handle.ReadRhoProjection()

	if err != nil {
		return fieldProjection{}, fmt.Errorf("manifold: read rho projection: %w", err)
	}

	pilotWave, err := handle.ReadPilotWaveProjection()

	if err != nil {
		return fieldProjection{}, fmt.Errorf("manifold: read pilot-wave projection: %w", err)
	}

	oscillators := make([]pmanifold.Oscillator, 0)

	if oscillatorCount > 0 {
		oscillators, err = handle.ReadOscillators(oscillatorCount)

		if err != nil {
			return fieldProjection{}, fmt.Errorf("manifold: read oscillators: %w", err)
		}
	}

	projection.Rho = rho
	projection.PsiMag2 = pilotWave.Mag2
	projection.GuidanceVelX = pilotWave.VelX
	projection.GuidanceVelZ = pilotWave.VelZ
	projection.Particles = particlesFromOscillators(oscillators, config)

	return projection, nil
}

/*
particlesFromOscillators maps post-step oscillator state into dashboard carriers
using the same toroidal cell indexing the GPU solver uses.
*/
func particlesFromOscillators(
	oscillators []pmanifold.Oscillator,
	config pmanifold.Config,
) []pmanifold.Particle {
	particles := make([]pmanifold.Particle, 0, len(oscillators))

	for _, oscillator := range oscillators {
		if oscillator.Amplitude <= 0 {
			continue
		}

		particles = append(particles, pmanifold.Particle{
			Role:      "whale_carrier",
			CellX:     cellCoordinate(oscillator.PosX, config.DomainX, config.GridX),
			CellY:     cellCoordinate(oscillator.PosY, config.DomainY, config.GridY),
			CellZ:     cellCoordinate(oscillator.PosZ, config.DomainZ, config.GridZ),
			Phase:     oscillator.Phase,
			Omega:     oscillator.Omega,
			Amplitude: oscillator.Amplitude,
			Heat:      oscillator.Heat,
			VelX:      oscillator.VelX,
			VelY:      oscillator.VelY,
			VelZ:      oscillator.VelZ,
			Speed: math.Sqrt(
				oscillator.VelX*oscillator.VelX +
					oscillator.VelY*oscillator.VelY +
					oscillator.VelZ*oscillator.VelZ,
			),
		})
	}

	return particles
}

func cellCoordinate(position, domain float64, grid uint32) float64 {
	if grid == 0 || domain <= 0 {
		return 0
	}

	index := int(math.Floor(position * float64(grid) / domain))

	if index < 0 {
		index = 0
	}

	maxIndex := int(grid) - 1

	if index > maxIndex {
		index = maxIndex
	}

	return float64(index)
}
