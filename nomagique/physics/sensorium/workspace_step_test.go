//go:build (darwin && cgo) || (linux && cuda && cgo)

package sensorium

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStep(t *testing.T) {
	Convey("The captured 2026-10-02 live boot population advances repeatedly", t, func() {
		state := capturedBootState(t)
		fluid, err := newWorkspace(64, 64, 64)
		So(err, ShouldBeNil)
		Reset(fluid.Close)
		fluid.loadState(state)

		for range 3 {
			reading, err := fluid.step()
			So(err, ShouldBeNil)
			So(reading.Health.Integrator.AcceptedDT, ShouldBeGreaterThan, 0)
			if reading.Health.Integrator.Rejections == 0 {
				So(reading.Health.Integrator.Substeps, ShouldEqual, 1)
			}
			So(reading.Health.Remap.MaxMarginalResidual, ShouldBeLessThanOrEqualTo, fluid.physics.RemapTolerance)
			fluid.storeState(state)
		}
	})

	Convey("A sparse resident population advances coupled physics repeatedly", t, func() {
		fluid := remapWorkspace(t, 64, 16)
		state := newState(16)
		copy(state.Pos, fluid.pos.Float32Slice())
		copy(state.Mass, fluid.mass.Float32Slice())
		copy(state.Heat, fluid.heat.Float32Slice())

		for particle := range state.N {
			state.ContentIDs[particle] = int64(particle + 1)
			state.Energy[particle] = 1
			state.Phase[particle] = float32(particle) * 2 * math.Pi / float32(state.N)
			state.Omega[particle] = float32(particle)/float32(state.N) - 0.5
		}
		fluid.loadState(state)

		for range 3 {
			reading, err := fluid.step()
			So(err, ShouldBeNil)
			So(reading.Health.Integrator.AcceptedDT, ShouldBeGreaterThan, 0)
			if reading.Health.Integrator.Rejections == 0 {
				So(reading.Health.Integrator.Substeps, ShouldEqual, 1)
			}
			fluid.storeState(state)
		}
	})
}

// capturedBootState is the actual projector output captured before a failed
// live 64^3 advance on 2026-10-02. No particle values have been synthesized.
func capturedBootState(t testing.TB) *State {
	t.Helper()
	payload, err := os.ReadFile("testdata/market_boot.json")
	if err != nil {
		t.Fatal(err)
	}
	state := &State{}
	if err := json.Unmarshal(payload, state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestSeedModeAnchors(t *testing.T) {
	Convey("Given two particles in the lowest ω bin", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() {
			fluid.Close()
		})
		fluid.allocateParticles(2)
		fluid.particles = 2
		omega := fluid.omega.Float32Slice()
		amp := fluid.amp.Float32Slice()
		omega[0] = float32(fluid.domain.OmegaMin)
		omega[1] = float32(fluid.domain.OmegaMin)
		amp[0] = 1
		amp[1] = 3
		fluid.seedModeAnchors()
		idx := fluid.anchorIdx.Int32Slice()
		weight := fluid.anchorWeight.Float32Slice()

		Convey("The stronger amplitude should occupy the first slot", func() {
			So(idx[0], ShouldEqual, int32(1))
			So(weight[0], ShouldEqual, float32(3))
			So(idx[1], ShouldEqual, int32(0))
			So(weight[1], ShouldEqual, float32(1))
		})
	})
}

func TestProjectSpatialWave(t *testing.T) {
	Convey("Spatial projection uses the periodic thermal overlap at every cell", t, func() {
		fluid := remapWorkspace(t, 8, 1)
		fluid.pos.Float32Slice()[0] = 0.5
		fluid.pos.Float32Slice()[1] = 0.5
		fluid.pos.Float32Slice()[2] = 0.5
		fluid.psiModeReal.Float32Slice()[0] = 1
		fluid.psiModeImag.Float32Slice()[0] = 2
		fluid.anchorIdx.Int32Slice()[0] = 0
		fluid.anchorWeight.Float32Slice()[0] = 1

		Convey("The zero-temperature limit has uniform support", func() {
			fluid.heat.Float32Slice()[0] = 0
			So(fluid.projectSpatialWave(), ShouldBeNil)
			for cell := range fluid.domain.CellCount() {
				So(fluid.psiRe.Float32Slice()[cell], ShouldAlmostEqual, 1)
				So(fluid.psiIm.Float32Slice()[cell], ShouldAlmostEqual, 2)
			}
		})

		Convey("Finite temperature resolves locality without leaving holes between anchors", func() {
			fluid.heat.Float32Slice()[0] = 32 // sigma = hbar / sqrt(2 m kB T) = 1/8.
			So(fluid.projectSpatialWave(), ShouldBeNil)
			center := 4*8*8 + 4*8 + 4
			So(fluid.psiRe.Float32Slice()[center], ShouldAlmostEqual, 1)
			// Independent image-sum value at distance L/2 in all three dimensions.
			numerator, denominator := 0.0, 0.0
			for image := -8; image <= 8; image++ {
				numerator += math.Exp(-math.Pow((0.5+float64(image))/0.25, 2))
				denominator += math.Exp(-math.Pow(float64(image)/0.25, 2))
			}
			So(float64(fluid.psiRe.Float32Slice()[0]), ShouldAlmostEqual, math.Pow(numerator/denominator, 3), 1e-9)
			for cell := range fluid.domain.CellCount() {
				So(fluid.psiRe.Float32Slice()[cell], ShouldBeGreaterThan, 0)
				So(fluid.psiIm.Float32Slice()[cell], ShouldAlmostEqual, 2*fluid.psiRe.Float32Slice()[cell])
			}
		})
	})
}

func TestWaveStep(t *testing.T) {
	Convey("Given one localized mode with no oscillator drive", t, func() {
		fluid, err := newWorkspace(64, 64, 64)
		So(err, ShouldBeNil)
		Reset(func() {
			fluid.Close()
		})
		fluid.allocateParticles(1)
		fluid.particles = 1
		fluid.mass.Float32Slice()[0] = 1
		fluid.materialEnergy.Float32Slice()[0] = 10
		fluid.psiRealHeads[0].Float32Slice()[fluid.domain.MaxModes/2] = 1
		steps := int(math.Ceil(
			1 / (fluid.rates.energyDecay * fluid.rates.deltaT),
		))

		for range steps {
			So(fluid.waveStep(), ShouldBeNil)
		}

		norm := 0.0

		for head := 0; head < spectralHeads; head++ {
			real := fluid.psiRealHeads[head].Float32Slice()
			imaginary := fluid.psiImagHeads[head].Float32Slice()

			for mode, value := range real {
				norm += float64(value)*float64(value) +
					float64(imaginary[mode])*float64(imaginary[mode])
			}
		}

		expected := math.Exp(
			-2 * fluid.rates.energyDecay * fluid.rates.deltaT * float64(steps),
		)

		Convey("The unitary kinetic evolution preserves the norm lost only to configured damping", func() {
			So(math.IsNaN(norm), ShouldBeFalse)
			So(math.IsInf(norm, 0), ShouldBeFalse)
			So(norm, ShouldAlmostEqual, expected, 1e-4)
		})
	})
}

func TestKuramotoFromPhase(t *testing.T) {
	Convey("Given two active antipodal phases in a larger capacity buffer", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() {
			fluid.Close()
		})
		fluid.allocateParticles(2)
		phase := fluid.phase.Float32Slice()
		phase[0] = 0
		phase[1] = math.Pi

		Convey("Only active oscillators contribute to the order parameter", func() {
			r, _ := kuramotoFromPhase(fluid.phase, 2)
			So(r, ShouldAlmostEqual, 0, 1e-6)
		})
	})
}

func TestSpatialSigma(t *testing.T) {
	Convey("Given particles whose seeding leaves zero mean temperature", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() {
			fluid.Close()
		})
		fluid.allocateParticles(1)
		fluid.particles = 1
		fluid.mass.Float32Slice()[0] = 1
		fluid.heat.Float32Slice()[0] = 0

		Convey("The coupling length approaches zero at the cold uniform limit", func() {
			sigma, err := fluid.spatialSigma()
			So(err, ShouldBeNil)
			So(math.IsNaN(sigma), ShouldBeFalse)
			So(math.IsInf(sigma, 0), ShouldBeFalse)
			So(sigma, ShouldEqual, 0)
			So(fluid.health.SigmaUniformLimit, ShouldBeTrue)
		})
	})

	Convey("Given a fully determined thermal mass", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() {
			fluid.Close()
		})
		fluid.allocateParticles(1)
		fluid.particles = 1
		fluid.mass.Float32Slice()[0] = 1
		fluid.heat.Float32Slice()[0] = 1

		Convey("The coupling length is a finite interior point", func() {
			sigma, err := fluid.spatialSigma()
			So(err, ShouldBeNil)
			So(math.IsNaN(sigma), ShouldBeFalse)
			So(math.IsInf(sigma, 0), ShouldBeFalse)
			So(sigma, ShouldBeGreaterThan, fluid.domain.GridSpacing())
		})
	})
}

func TestGatherPilotWave(t *testing.T) {
	Convey("Given a periodic plane wave and particles of different masses", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() { fluid.Close() })
		fluid.allocateParticles(2)
		fluid.particles = 2
		fluid.mass.Float32Slice()[0], fluid.mass.Float32Slice()[1] = 1, 2
		positions := fluid.pos.Float32Slice()
		copy(positions, []float32{0.5, 0.5, 0.5, 0.5, 0.5, 0.5})
		real, imag := fluid.psiRe.Float32Slice(), fluid.psiIm.Float32Slice()

		for cell := range real {
			phase := 2 * math.Pi * float64(cell/(8*8)) / 8
			real[cell], imag[cell] = float32(math.Cos(phase)), float32(math.Sin(phase))
		}
		copy(fluid.psiStartRe.Float32Slice(), real)
		copy(fluid.psiStartIm.Float32Slice(), imag)
		fluid.materialEnergy.Float32Slice()[0] = 100
		fluid.materialEnergy.Float32Slice()[1] = 100
		fluid.rates.deltaT = 0.005

		So(fluid.gatherPilotWave(), ShouldBeNil)
		velocity := fluid.vel.Float32Slice()
		expected := float64(velocity[0])
		So(expected, ShouldBeGreaterThan, 6.0)
		So(float64(velocity[3]), ShouldAlmostEqual, expected/2, 0.2)
		So(velocity[1], ShouldAlmostEqual, 0)
		So(velocity[2], ShouldAlmostEqual, 0)
		So(float64(positions[0]), ShouldAlmostEqual, 0.5+expected*fluid.rates.deltaT, 1e-2)

		Convey("Conjugating the wave reverses its current", func() {
			for cell := range imag {
				imag[cell] = -imag[cell]
			}

			fluid.gatherPilotWave()
			So(velocity[0], ShouldBeLessThan, 0)
			So(velocity[3], ShouldBeLessThan, 0)
		})

		Convey("A zero field is rejected as an exact node without modifying position", func() {
			before := append([]float32(nil), positions[:6]...)
			clear(real)
			clear(imag)
			clear(fluid.psiStartRe.Float32Slice())
			clear(fluid.psiStartIm.Float32Slice())
			So(fluid.gatherPilotWave(), ShouldNotBeNil)
			So(positions[:6], ShouldResemble, before)
		})
	})
}

func TestPlanckExchange(t *testing.T) {
	Convey("Given particles on opposite sides of thermal equilibrium", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() { fluid.Close() })
		fluid.allocateParticles(2)
		fluid.particles = 2
		copy(fluid.mass.Float32Slice(), []float32{1, 2})
		copy(fluid.omega.Float32Slice(), []float32{1, 2})
		copy(fluid.heat.Float32Slice(), []float32{10, 0})
		copy(fluid.oscEnergy.Float32Slice(), []float32{0, 10})

		copy(fluid.materialEnergy.Float32Slice(), []float32{10, 10})

		Convey("Repeated exchange conserves energy and evolves the oscillator amplitudes", func() {
			for range 5 {
				So(fluid.planckExchange(), ShouldBeNil)

				for index := 0; index < fluid.particles; index++ {
					energy := fluid.oscEnergy.Float32Slice()[index]
					heat := fluid.heat.Float32Slice()[index]
					amp := fluid.amp.Float32Slice()[index]
					So(float64(energy+heat), ShouldAlmostEqual, 10, 1e-5)
					So(energy, ShouldBeGreaterThanOrEqualTo, 0)
					So(heat, ShouldBeGreaterThanOrEqualTo, 0)
					So(float64(amp*amp), ShouldAlmostEqual, float64(energy), 1e-5)
				}
			}

			So(fluid.oscEnergy.Float32Slice()[0], ShouldBeGreaterThan, 0)
			So(fluid.heat.Float32Slice()[1], ShouldBeGreaterThan, 0)
		})

		Convey("Negative thermal energy returns a descriptive error", func() {
			fluid.heat.Float32Slice()[0] = -1
			err := fluid.planckExchange()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid reservoir")
		})

		Convey("Negative oscillator energy returns a descriptive error", func() {
			fluid.oscEnergy.Float32Slice()[0] = -1
			err := fluid.planckExchange()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid reservoir")
		})
	})
}

func BenchmarkWaveStep(b *testing.B) {
	fluid, err := newWorkspace(64, 64, 64)

	if err != nil {
		b.Fatal(err)
	}

	defer fluid.Close()
	fluid.allocateParticles(1)
	fluid.particles = 1
	fluid.mass.Float32Slice()[0] = 1
	fluid.heat.Float32Slice()[0] = 1
	fluid.oscEnergy.Float32Slice()[0] = 1
	fluid.amp.Float32Slice()[0] = 1
	fluid.seedModeAnchors()
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		fluid.waveStep()
	}
}

func BenchmarkGatherPilotWave(b *testing.B) {
	fluid, err := newWorkspace(8, 8, 8)

	if err != nil {
		b.Fatal(err)
	}

	defer fluid.Close()
	// A full 100-level book on each side.
	fluid.allocateParticles(200)
	fluid.particles = 200

	for index := 0; index < fluid.particles; index++ {
		fluid.mass.Float32Slice()[index] = 1
	}

	for cell := range fluid.psiRe.Float32Slice() {
		phase := 2 * math.Pi * float64(cell/(8*8)) / 8
		fluid.psiRe.Float32Slice()[cell] = float32(math.Cos(phase))
		fluid.psiIm.Float32Slice()[cell] = float32(math.Sin(phase))
	}

	b.ReportAllocs()

	for b.Loop() {
		fluid.gatherPilotWave()
	}
}

func BenchmarkPlanckExchange(b *testing.B) {
	fluid, err := newWorkspace(8, 8, 8)

	if err != nil {
		b.Fatal(err)
	}

	defer fluid.Close()
	fluid.allocateParticles(200)
	fluid.particles = 200

	for index := 0; index < fluid.particles; index++ {
		fluid.mass.Float32Slice()[index] = 1
		fluid.omega.Float32Slice()[index] = 1
		fluid.heat.Float32Slice()[index] = 10
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := fluid.planckExchange(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStep(b *testing.B) {
	state := capturedBootState(b)
	fluid, err := newWorkspace(64, 64, 64)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(fluid.Close)
	fluid.loadState(state)
	b.ResetTimer()
	for b.Loop() {
		if _, err := fluid.step(); err != nil {
			b.Fatal(err)
		}
		fluid.storeState(state)
	}
}
