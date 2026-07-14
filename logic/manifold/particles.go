package manifold

import (
	"math"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

const whaleStandoutMAD = 1.5

type oscillatorReader interface {
	ReadOscillators(count int) ([]pmanifold.Oscillator, error)
}

/*
particlesFromOscillators maps post-step solver readback into the terminal particle
contract. Whale carriers are amplitude standouts relative to the live oscillator
pack, not a fixed mass cutoff.
*/
func particlesFromOscillators(
	config *pmanifold.Config,
	oscillators []pmanifold.Oscillator,
) []pmanifold.Particle {
	if config == nil || len(oscillators) == 0 {
		return nil
	}

	amplitudes := make([]float64, len(oscillators))

	for index, oscillator := range oscillators {
		amplitudes[index] = oscillator.Amplitude
	}

	center := median(amplitudes)
	dispersion := medianAbsoluteDeviation(amplitudes, center)
	whaleThreshold := center + whaleStandoutMAD*dispersion
	particles := make([]pmanifold.Particle, 0, len(oscillators))

	for _, oscillator := range oscillators {
		role := "carrier"

		if oscillator.Amplitude > whaleThreshold {
			role = "whale_carrier"
		}

		speed := math.Hypot(
			oscillator.VelX,
			math.Hypot(oscillator.VelY, oscillator.VelZ),
		)

		particles = append(particles, pmanifold.Particle{
			Role:      role,
			CellX:     domainCell(oscillator.PosX, config.DomainX, config.GridX),
			CellY:     domainCell(oscillator.PosY, config.DomainY, config.GridY),
			CellZ:     domainCell(oscillator.PosZ, config.DomainZ, config.GridZ),
			Phase:     oscillator.Phase,
			Omega:     oscillator.Omega,
			Amplitude: oscillator.Amplitude,
			Heat:      oscillator.Heat,
			VelX:      oscillator.VelX,
			VelY:      oscillator.VelY,
			VelZ:      oscillator.VelZ,
			Speed:     speed,
		})
	}

	return particles
}

/*
readParticles captures post-step oscillator positions from the Metal solver.
*/
func readParticles(
	config *pmanifold.Config,
	solver oscillatorReader,
	count int,
) ([]pmanifold.Particle, error) {
	if count <= 0 {
		return nil, nil
	}

	oscillators, err := solver.ReadOscillators(count)

	if err != nil {
		return nil, err
	}

	return particlesFromOscillators(config, oscillators), nil
}

func domainCell(position float64, domain float64, grid uint32) float64 {
	if domain <= 0 || grid == 0 {
		return 0
	}

	normalized := position / domain

	if normalized < 0 {
		normalized = 0
	}

	if normalized >= 1 {
		normalized = math.Nextafter(1, 0)
	}

	cell := uint32(normalized * float64(grid))

	if cell >= grid {
		cell = grid - 1
	}

	return float64(cell)
}

func medianAbsoluteDeviation(values []float64, center float64) float64 {
	if len(values) == 0 {
		return 0
	}

	deviations := make([]float64, len(values))

	for index, value := range values {
		deviations[index] = math.Abs(value - center)
	}

	return median(deviations)
}
