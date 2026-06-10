//go:build darwin && cgo

package physics

import (
	"fmt"
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestSolverStep(t *testing.T) {
	convey.Convey("Given a Metal manifold solver", t, func() {
		config := Config{
			GridX:    8,
			GridY:    1,
			GridZ:    8,
			DomainX:  0.16,
			DomainY:  1,
			DomainZ:  8,
			DeltaT:   0.1,
			Gamma:    5.0 / 3.0,
			CV:       0.5,
			RhoMin:   1e-3,
			PMin:     1e-6,
			KThermal: 1e-2,
			MaxModes: 4,
		}

		solver, err := NewSolver(config)

		convey.Convey("It should accept deposits, oscillators, and return finite readings", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(solver, convey.ShouldNotBeNil)

			defer solver.Close()

			convey.So(solver.ResetDeposits(), convey.ShouldBeNil)

			for cellX := uint32(0); cellX < config.GridX; cellX++ {
				for cellZ := uint32(0); cellZ < config.GridZ; cellZ++ {
					convey.So(solver.DepositCell(cellX, 0, cellZ, 0.05, 0, 0, 0, 0.05), convey.ShouldBeNil)
				}
			}

			convey.So(solver.SetOscillators([]Oscillator{{
				Phase:     0.5,
				Omega:     6.28,
				Amplitude: 0.2,
				PosX:      0.4,
				PosY:      0,
				PosZ:      1.2,
				Heat:      0.2,
				VelX:      0.4,
			}}), convey.ShouldBeNil)

			reading, stepErr := solver.Step()

			convey.So(stepErr, convey.ShouldBeNil)
			convey.So(math.IsNaN(reading.PressureGradNorm), convey.ShouldBeFalse)
			convey.So(math.IsInf(reading.CoherenceMag2, 0), convey.ShouldBeFalse)
			convey.So(reading.CoherenceMag2, convey.ShouldBeGreaterThan, 0)

			rho, rhoErr := solver.ReadRhoProjection()

			convey.So(rhoErr, convey.ShouldBeNil)
			convey.So(len(rho), convey.ShouldEqual, int(config.GridZ))
			convey.So(len(rho[0]), convey.ShouldEqual, int(config.GridX))
		})
	})
}

func TestReadProjectionReading(t *testing.T) {
	convey.Convey("Given a deposited rho lattice", t, func() {
		config := Config{
			GridX:    8,
			GridY:    1,
			GridZ:    8,
			DomainX:  0.16,
			DomainY:  1,
			DomainZ:  8,
			DeltaT:   0.1,
			Gamma:    5.0 / 3.0,
			CV:       0.5,
			RhoMin:   1e-3,
			PMin:     1e-6,
			KThermal: 1e-2,
			MaxModes: 4,
		}

		solver, err := NewSolver(config)

		convey.Convey("It should derive bulk observables from the rho projection", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(solver, convey.ShouldNotBeNil)

			defer solver.Close()

			convey.So(solver.ResetDeposits(), convey.ShouldBeNil)
			convey.So(solver.DepositCell(4, 0, 4, 1, 0, 0, 0, 1), convey.ShouldBeNil)
			convey.So(solver.SetOscillators([]Oscillator{{
				Amplitude: 0.2,
				Heat:      0.2,
			}}), convey.ShouldBeNil)
			_, stepErr := solver.Step()

			convey.So(stepErr, convey.ShouldBeNil)

			reading, projectionErr := solver.ReadProjectionReading()

			convey.So(projectionErr, convey.ShouldBeNil)
			convey.So(reading.PressureGradNorm, convey.ShouldBeGreaterThan, 0)
			convey.So(reading.Divergence, convey.ShouldBeGreaterThan, 0)
			convey.So(reading.ViscosityProxy, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestReadOscillators(t *testing.T) {
	convey.Convey("Given a stepped solver with oscillators", t, func() {
		config := Config{
			GridX:    8,
			GridY:    1,
			GridZ:    8,
			DomainX:  0.16,
			DomainY:  1,
			DomainZ:  8,
			DeltaT:   0.1,
			Gamma:    5.0 / 3.0,
			CV:       0.5,
			RhoMin:   1e-3,
			PMin:     1e-6,
			KThermal: 1e-2,
			MaxModes: 4,
		}

		solver, err := NewSolver(config)

		convey.Convey("It should read post-step particle state from Metal", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(solver, convey.ShouldNotBeNil)

			defer solver.Close()

			convey.So(solver.ResetDeposits(), convey.ShouldBeNil)
			convey.So(solver.DepositCell(4, 0, 4, 0.5, 0, 0, 0, 0.5), convey.ShouldBeNil)
			convey.So(solver.SetOscillators([]Oscillator{{
				Phase:     0.5,
				Omega:     6.28,
				Amplitude: 0.2,
				PosX:      0.4,
				PosY:      0,
				PosZ:      1.2,
				Heat:      0.2,
				VelX:      0.4,
			}}), convey.ShouldBeNil)

			_, stepErr := solver.Step()

			convey.So(stepErr, convey.ShouldBeNil)

			oscillators, readErr := solver.ReadOscillators(1)

			convey.So(readErr, convey.ShouldBeNil)
			convey.So(len(oscillators), convey.ShouldEqual, 1)
			convey.So(oscillators[0].Heat, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestSolverWhaleParticleVelocity(t *testing.T) {
	convey.Convey("Given a Metal manifold solver", t, func() {
		config := Config{
			GridX:    8,
			GridY:    1,
			GridZ:    8,
			DomainX:  0.16,
			DomainY:  1,
			DomainZ:  8,
			DeltaT:   0.1,
			Gamma:    5.0 / 3.0,
			CV:       0.5,
			RhoMin:   1e-3,
			PMin:     1e-6,
			KThermal: 1e-2,
			MaxModes: 4,
		}

		solver, err := NewSolver(config)

		convey.Convey("It should step with whale particles carrying directional velocity", func() {
			convey.So(err, convey.ShouldBeNil)
			defer solver.Close()

			convey.So(solver.ResetDeposits(), convey.ShouldBeNil)

			for cellX := uint32(0); cellX < config.GridX; cellX++ {
				for cellZ := uint32(0); cellZ < config.GridZ; cellZ++ {
					convey.So(solver.DepositCell(cellX, 0, cellZ, 0.05, 0, 0, 0, 0.05), convey.ShouldBeNil)
				}
			}

			convey.So(solver.SetOscillators([]Oscillator{{
				Phase:     0.5,
				Omega:     6.28,
				Amplitude: 0.2,
				PosX:      0.4,
				PosY:      0,
				PosZ:      1.2,
				Heat:      0.2,
				VelX:      0.4,
			}}), convey.ShouldBeNil)

			reading, stepErr := solver.Step()

			convey.So(stepErr, convey.ShouldBeNil)
			convey.So(math.IsNaN(reading.PressureGradNorm), convey.ShouldBeFalse)
			convey.So(math.IsInf(reading.CoherenceMag2, 0), convey.ShouldBeFalse)
			convey.So(reading.CoherenceMag2, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestSolverProductionConfig(t *testing.T) {
	convey.Convey("Given production manifold grid dimensions", t, func() {
		config := Config{
			GridX:    32,
			GridY:    3,
			GridZ:    16,
			DomainX:  0.32,
			DomainY:  3,
			DomainZ:  16,
			DeltaT:   0.1,
			Gamma:    5.0 / 3.0,
			CV:       0.5,
			RhoMin:   1e-3,
			PMin:     1e-6,
			KThermal: 1e-2,
			MaxModes: 32,
		}

		solver, err := NewSolver(config)

		convey.Convey("It should step with 32 oscillators on a 32x3x16 grid", func() {
			convey.So(err, convey.ShouldBeNil)
			defer solver.Close()

			convey.So(solver.ResetDeposits(), convey.ShouldBeNil)

			for cellX := uint32(0); cellX < config.GridX; cellX++ {
				for cellY := uint32(0); cellY < config.GridY; cellY++ {
					for cellZ := uint32(0); cellZ < config.GridZ; cellZ++ {
						convey.So(solver.DepositCell(cellX, cellY, cellZ, 0.05, 0, 0, 0, 0.05), convey.ShouldBeNil)
					}
				}
			}

			oscillators := make([]Oscillator, config.MaxModes)

			for index := range oscillators {
				oscillators[index] = Oscillator{
					Phase:     float64(index) * 0.1,
					Omega:     6.28,
					Amplitude: 0.1,
					PosX:      float64(index % int(config.GridX)),
					PosY:      float64(index % int(config.GridY)),
					PosZ:      float64(index % int(config.GridZ)),
					Heat:      0.1,
				}
			}

			convey.So(solver.SetOscillators(oscillators), convey.ShouldBeNil)

			reading, stepErr := solver.Step()

			convey.So(stepErr, convey.ShouldBeNil)
			convey.So(math.IsNaN(reading.PressureGradNorm), convey.ShouldBeFalse)
			convey.So(math.IsInf(reading.CoherenceMag2, 0), convey.ShouldBeFalse)

			rho, rhoErr := solver.ReadRhoProjection()

			convey.So(rhoErr, convey.ShouldBeNil)
			convey.So(len(rho), convey.ShouldEqual, int(config.GridZ))
			convey.So(len(rho[0]), convey.ShouldEqual, int(config.GridX))
		})
	})
}

func TestSolverCarrierThreshold(t *testing.T) {
	config := productionTestConfig()
	deltaT := config.DeltaT

	for _, count := range []int{32, 48, 64, 96, 128} {
		t.Run(fmt.Sprintf("count=%d", count), func(t *testing.T) {
			solver, err := NewSolver(config)

			if err != nil {
				t.Fatal(err)
			}

			defer solver.Close()

			if err := solver.ResetDeposits(); err != nil {
				t.Fatal(err)
			}

			if err := solver.DepositCell(1, 0, 1, 0.05, 0, 0, 0, 0.05); err != nil {
				t.Fatal(err)
			}

			omega := 2 * math.Pi / deltaT
			osc := make([]Oscillator, count)

			for index := range osc {
				perCarrierEnergy := config.RhoMin / float64(count)
				osc[index] = Oscillator{
					Phase:     float64(index) * 0.1,
					Omega:     omega,
					Amplitude: math.Sqrt(perCarrierEnergy),
					Heat:      perCarrierEnergy,
					PosX:      1,
					PosY:      0,
					PosZ:      1,
				}
			}

			if err := solver.SetOscillators(osc); err != nil {
				t.Fatal(err)
			}

			if _, err := solver.Step(); err != nil {
				t.Fatal(err)
			}

			read, err := solver.ReadOscillators(count)

			if err != nil {
				t.Fatal(err)
			}

			if math.IsNaN(read[0].Phase) {
				t.Fatalf("phase NaN at count %d", count)
			}
		})
	}
}

func TestSolverProduction128Oscillators(t *testing.T) {
	convey.Convey("Given production rho_min and 128 startup oscillators", t, func() {
		config := productionTestConfig()
		carrierCount := 128

		solver, err := NewSolver(config)

		convey.Convey("It should return finite oscillator readback after step", func() {
			convey.So(err, convey.ShouldBeNil)
			defer solver.Close()

			convey.So(solver.ResetDeposits(), convey.ShouldBeNil)
			convey.So(
				solver.DepositCell(1, 0, 1, 0.05, 0, 0, 0, 0.05),
				convey.ShouldBeNil,
			)

			omega := 2 * math.Pi / config.DeltaT
			oscillators := make([]Oscillator, carrierCount)

			for index := range oscillators {
				perCarrierEnergy := config.RhoMin / float64(carrierCount)
				oscillators[index] = Oscillator{
					Phase:     float64(index) * 0.1,
					Omega:     omega,
					Amplitude: math.Sqrt(perCarrierEnergy),
					PosX:      1,
					PosY:      0,
					PosZ:      1,
					Heat:      perCarrierEnergy,
				}
			}

			convey.So(solver.SetOscillators(oscillators), convey.ShouldBeNil)

			_, stepErr := solver.Step()

			convey.So(stepErr, convey.ShouldBeNil)

			readback, readErr := solver.ReadOscillators(len(oscillators))

			convey.So(readErr, convey.ShouldBeNil)
			convey.So(len(readback), convey.ShouldEqual, carrierCount)
			convey.So(math.IsNaN(readback[0].Phase), convey.ShouldBeFalse)
			convey.So(math.IsNaN(readback[0].Heat), convey.ShouldBeFalse)
			convey.So(math.IsNaN(readback[0].Amplitude), convey.ShouldBeFalse)
		})
	})
}

func productionTestConfig() Config {
	tickSize := 0.01
	halfWidth := 32
	gamma := 5.0 / 3.0
	deltaT := 0.1

	config := Config{
		GridX:    32,
		GridY:    3,
		GridZ:    16,
		DomainX:  float64(halfWidth*2+1) * tickSize,
		DomainY:  3,
		DomainZ:  16,
		DeltaT:   deltaT,
		Gamma:    gamma,
		MaxModes: 128,
	}

	cellVolume := config.CellVolume()
	rhoMin := 1.0 / cellVolume

	config.CV = 1.0 / (gamma - 1.0)
	config.RhoMin = rhoMin
	config.PMin = (gamma - 1.0) * rhoMin * cellVolume
	config.KThermal = rhoMin / deltaT

	return config
}

func TestSolverMultiSymbolDeposits(t *testing.T) {
	config := productionTestConfig()
	carrierCount := 128

	solver, err := NewSolver(config)

	if err != nil {
		t.Fatal(err)
	}

	defer solver.Close()

	if err := solver.ResetDeposits(); err != nil {
		t.Fatal(err)
	}

	for symbolIndex := 0; symbolIndex < carrierCount; symbolIndex++ {
		rho := 0.05 / float64(carrierCount)

		if depositErr := solver.DepositCell(1, 0, 1, rho, rho, 0, 0, rho*config.CV); depositErr != nil {
			t.Fatal(depositErr)
		}
	}

	omega := 2 * math.Pi / config.DeltaT
	oscillators := make([]Oscillator, carrierCount)

	for index := range oscillators {
		perCarrierEnergy := config.RhoMin / float64(carrierCount)
		oscillators[index] = Oscillator{
			Phase:     float64(index) * 0.1,
			Omega:     omega,
			Amplitude: math.Sqrt(perCarrierEnergy),
			PosX:      1,
			PosY:      0,
			PosZ:      1,
			Heat:      perCarrierEnergy,
		}
	}

	if err := solver.SetOscillators(oscillators); err != nil {
		t.Fatal(err)
	}

	if _, err := solver.Step(); err != nil {
		t.Fatal(err)
	}

	readback, err := solver.ReadOscillators(carrierCount)

	if err != nil {
		t.Fatal(err)
	}

	if math.IsNaN(readback[0].Phase) {
		t.Fatalf("phase NaN with scaled multi-symbol deposits")
	}
}

func BenchmarkSolverStep(b *testing.B) {
	config := Config{
		GridX:    16,
		GridY:    1,
		GridZ:    16,
		DomainX:  0.32,
		DomainY:  1,
		DomainZ:  16,
		DeltaT:   0.1,
		Gamma:    5.0 / 3.0,
		CV:       0.5,
		RhoMin:   1e-3,
		PMin:     1e-6,
		KThermal: 1e-2,
		MaxModes: 8,
	}

	solver, err := NewSolver(config)

	if err != nil {
		b.Fatal(err)
	}

	defer solver.Close()

	oscillators := make([]Oscillator, 8)

	for index := range oscillators {
		oscillators[index] = Oscillator{
			Phase:     float64(index) * 0.1,
			Omega:     6.28,
			Amplitude: 0.1,
			PosX:      float64(index),
			PosY:      0,
			PosZ:      float64(index),
			Heat:      0.1,
		}
	}

	if err := solver.SetOscillators(oscillators); err != nil {
		b.Fatal(err)
	}

	if err := solver.DepositCell(8, 0, 4, 1, 0.2, 0, 0, 0.5); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := solver.ResetDeposits(); err != nil {
			b.Fatal(err)
		}

		if err := solver.DepositCell(8, 0, 4, 1, 0.2, 0, 0, 0.5); err != nil {
			b.Fatal(err)
		}

		if _, err := solver.Step(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestMaxModesVsOscCount(t *testing.T) {
	for _, testCase := range []struct {
		maxModes int
		numOsc   int
	}{
		{32, 32},
		{128, 32},
		{128, 128},
	} {
		t.Run(fmt.Sprintf("max=%d-osc=%d", testCase.maxModes, testCase.numOsc), func(t *testing.T) {
			config := productionTestConfig()
			config.MaxModes = uint32(testCase.maxModes)

			solver, err := NewSolver(config)

			if err != nil {
				t.Fatal(err)
			}

			defer solver.Close()

			if err := solver.ResetDeposits(); err != nil {
				t.Fatal(err)
			}

			if err := solver.DepositCell(1, 0, 1, 0.05, 0, 0, 0, 0.05); err != nil {
				t.Fatal(err)
			}

			omega := 2 * math.Pi / config.DeltaT
			oscillators := make([]Oscillator, testCase.numOsc)

			for index := range oscillators {
				perCarrierEnergy := config.RhoMin / float64(testCase.numOsc)
				oscillators[index] = Oscillator{
					Phase:     float64(index) * 0.1,
					Omega:     omega,
					Amplitude: math.Sqrt(perCarrierEnergy),
					PosX:      1,
					PosY:      0,
					PosZ:      1,
					Heat:      perCarrierEnergy,
				}
			}

			if err := solver.SetOscillators(oscillators); err != nil {
				t.Fatal(err)
			}

			if _, err := solver.Step(); err != nil {
				t.Fatal(err)
			}

			readback, err := solver.ReadOscillators(testCase.numOsc)

			if err != nil {
				t.Fatal(err)
			}

			if math.IsNaN(readback[0].Phase) {
				t.Fatalf("phase NaN")
			}
		})
	}
}
