//go:build !darwin || !cgo

package physics

import "fmt"

/*
Solver runs the 3D PIC + GPE manifold on Metal (darwin + cgo only).
*/
type Solver struct{}

type Oscillator struct {
	Phase     float64
	Omega     float64
	Amplitude float64
	PosX      float64
	PosY      float64
	PosZ      float64
	Heat      float64
	VelX      float64
	VelY      float64
	VelZ      float64
}

func NewSolver(config Config) (*Solver, error) {
	return nil, fmt.Errorf("physics: Metal manifold solver requires darwin with cgo enabled")
}

func (solver *Solver) Close() {}

func (solver *Solver) ResetDeposits() error {
	return fmt.Errorf("physics: Metal manifold solver unavailable on this platform")
}

func (solver *Solver) DepositCell(
	cellX, cellY, cellZ uint32,
	rho, momX, momY, momZ, eInt float64,
) error {
	return fmt.Errorf("physics: Metal manifold solver unavailable on this platform")
}

func (solver *Solver) SetOscillators(oscillators []Oscillator) error {
	return fmt.Errorf("physics: Metal manifold solver unavailable on this platform")
}

func (solver *Solver) Step() (Reading, error) {
	return Reading{}, fmt.Errorf("physics: Metal manifold solver unavailable on this platform")
}

func (solver *Solver) ReadRhoProjection() ([][]float64, error) {
	return nil, fmt.Errorf("physics: Metal manifold solver unavailable on this platform")
}
