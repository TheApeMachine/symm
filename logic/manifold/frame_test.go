package manifold

import (
	"testing"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
)

func TestParticlesFromOscillators(t *testing.T) {
	config := pmanifold.Config{
		GridX:   16,
		GridY:   3,
		GridZ:   8,
		DomainX: 16,
		DomainY: 3,
		DomainZ: 8,
	}

	particles := particlesFromOscillators([]pmanifold.Oscillator{
		{
			Phase:     1.2,
			Omega:     2.5,
			Amplitude: 0.8,
			PosX:      8.5,
			PosY:      1.5,
			PosZ:      4.5,
			Heat:      0.3,
			VelX:      0.1,
			VelY:      0.2,
			VelZ:      0.3,
		},
	}, config)

	if len(particles) != 1 {
		t.Fatalf("particles = %d, want 1", len(particles))
	}

	particle := particles[0]

	if particle.Role != "whale_carrier" {
		t.Fatalf("role = %q, want whale_carrier", particle.Role)
	}

	if particle.CellX != 8 || particle.CellY != 1 || particle.CellZ != 4 {
		t.Fatalf("cells = (%v,%v,%v), want (8,1,4)", particle.CellX, particle.CellY, particle.CellZ)
	}

	if particle.Speed <= 0 {
		t.Fatalf("speed = %v, want finite positive", particle.Speed)
	}
}

func TestCellCoordinate(t *testing.T) {
	if got := cellCoordinate(8.5, 16, 16); got != 8 {
		t.Fatalf("cell coordinate = %v, want 8", got)
	}
}
