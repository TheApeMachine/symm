//go:build darwin && cgo

package physics

import "testing"

func tryStep(t *testing.T, config Config, numOsc uint32) {
	t.Helper()
	ApplyDerivedGasParams(&config)
	solver, err := NewSolver(config)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer solver.Close()
	if err := solver.ResetDeposits(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := solver.DepositCell(1, 0, 1, 0.05, 0, 0, 0, 0.05); err != nil {
		t.Fatalf("deposit: %v", err)
	}
	osc := make([]Oscillator, numOsc)
	for i := range osc {
		osc[i] = Oscillator{Omega: 6.28, Amplitude: 0.1, PosX: 1, PosY: 0, PosZ: 1, Heat: 0.1}
	}
	if err := solver.SetOscillators(osc); err != nil {
		t.Fatalf("osc: %v", err)
	}
	if _, err := solver.Step(); err != nil {
		t.Fatalf("step: %v", err)
	}
}

func TestBisectGridY3(t *testing.T) {
	tryStep(t, Config{GridX: 8, GridY: 3, GridZ: 8, DomainX: 0.16, DomainY: 3, DomainZ: 8, DeltaT: 0.1, Gamma: 5.0 / 3.0, MaxModes: 4}, 1)
}

func TestBisect32Osc(t *testing.T) {
	tryStep(t, Config{GridX: 8, GridY: 1, GridZ: 8, DomainX: 0.16, DomainY: 1, DomainZ: 8, DeltaT: 0.1, Gamma: 5.0 / 3.0, MaxModes: 32}, 32)
}

func TestBisectBigGrid(t *testing.T) {
	tryStep(t, Config{GridX: 32, GridY: 1, GridZ: 16, DomainX: 0.32, DomainY: 1, DomainZ: 16, DeltaT: 0.1, Gamma: 5.0 / 3.0, MaxModes: 32}, 32)
}

func TestBisectProductionLite(t *testing.T) {
	tryStep(t, Config{GridX: 32, GridY: 3, GridZ: 16, DomainX: 0.32, DomainY: 3, DomainZ: 16, DeltaT: 0.1, Gamma: 5.0 / 3.0, MaxModes: 32}, 32)
}
