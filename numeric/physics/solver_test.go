//go:build darwin && cgo

package physics

import (
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
