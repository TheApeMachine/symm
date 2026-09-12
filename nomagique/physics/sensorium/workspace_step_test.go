package sensorium

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

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
	Convey("Given one unit mode amplitude anchored to a particle at a cell centre", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() {
			fluid.Close()
		})
		fluid.allocateParticles(1)
		fluid.particles = 1
		pos := fluid.pos.Float32Slice()
		pos[0] = 0.5
		pos[1] = 0.5
		pos[2] = 0.5

		// projectSpatialWave carries MODE coefficients onto the grid through
		// their spatial anchors -- it does not deposit particle mass. The mode
		// amplitude and the anchor binding it to a particle are what this
		// function reads, and what the pipeline's earlier seedModeAnchors and
		// waveStep stages would otherwise have written.
		fluid.psiModeReal.Float32Slice()[0] = 1
		fluid.psiModeImag.Float32Slice()[0] = 0
		fluid.anchorIdx.Int32Slice()[0] = 0
		fluid.anchorWeight.Float32Slice()[0] = 1

		fluid.projectSpatialWave()
		psiRe := fluid.psiRe.Float32Slice()
		var total float32
		var peak float32
		var peakCell int

		for cell, value := range psiRe {
			total += value

			if value > peak {
				peak = value
				peakCell = cell
			}
		}

		Convey("CIC should deposit the mode phasor on the spatial grid", func() {
			So(float64(total), ShouldAlmostEqual, 1.0, 1e-5)
			So(peak, ShouldEqual, float32(1))
			So(peakCell, ShouldEqual, 4+8*(4+8*4))
		})
	})
}

func TestSplatParticleWave(t *testing.T) {
	Convey("Given a real-valued mode phasor anchored at a cell centre", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() {
			fluid.Close()
		})
		fluid.allocateParticles(1)
		fluid.particles = 1
		pos := fluid.pos.Float32Slice()
		pos[0] = 0.5
		pos[1] = 0.5
		pos[2] = 0.5
		fluid.psiModeReal.Float32Slice()[0] = 1
		fluid.psiModeImag.Float32Slice()[0] = 0
		fluid.anchorIdx.Int32Slice()[0] = 0
		fluid.anchorWeight.Float32Slice()[0] = 1
		fluid.psiRe.Zero()
		fluid.psiIm.Zero()
		fluid.projectSpatialWave()
		psiRe := fluid.psiRe.Float32Slice()
		var total float32
		var peak float32
		var peakCell int

		for cell, value := range psiRe {
			total += value

			if value > peak {
				peak = value
				peakCell = cell
			}
		}

		Convey("CIC should deposit the phasor and leave the imaginary part zero", func() {
			So(float64(total), ShouldAlmostEqual, 1.0, 1e-5)
			So(peak, ShouldEqual, float32(1))
			So(peakCell, ShouldEqual, 4+8*(4+8*4))
			So(fluid.psiIm.Float32Slice()[peakCell], ShouldEqual, float32(0))
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
			So(kuramotoFromPhase(fluid.phase, 2), ShouldAlmostEqual, 0, 1e-6)
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
