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
		fluid.psiRealHeads[0].Float32Slice()[fluid.domain.MaxModes/2] = 1
		steps := int(math.Ceil(
			1 / (fluid.rates.energyDecay * fluid.rates.deltaT),
		))

		for range steps {
			fluid.waveStep()
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

		Convey("The coupling length saturates instead of dividing by zero", func() {
			sigma := fluid.spatialSigma()

			So(math.IsNaN(sigma), ShouldBeFalse)
			So(math.IsInf(sigma, 0), ShouldBeFalse)
			So(sigma, ShouldBeGreaterThan, 0)
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
			sigma := fluid.spatialSigma()

			So(math.IsNaN(sigma), ShouldBeFalse)
			So(math.IsInf(sigma, 0), ShouldBeFalse)
			So(sigma, ShouldBeGreaterThan, fluid.domain.GridSpacing())
		})
	})
}

func TestGatherParticles(t *testing.T) {
	Convey("Given particles sampled through the production gather kernel", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() { fluid.Close() })
		fluid.allocateParticles(2)
		fluid.particles = 2

		for particle := range fluid.particles {
			fluid.mass.Float32Slice()[particle] = 1
			fluid.pos.Float32Slice()[particle*3] = 0.5
		}

		Convey("Exact vacuum and a populated field can alternate without rejection", func() {
			for _, density := range []float32{0, 1, 0, 2} {
				for cell := range fluid.rho.Float32Slice() {
					fluid.rho.Float32Slice()[cell] = density
					fluid.energy.Float32Slice()[cell] = density
				}

				So(fluid.gatherParticles(), ShouldBeNil)

				for particle := range fluid.particles {
					expectedHeat := float32(0)

					if density > 0 {
						expectedHeat = 1 // Unit mass and unit specific internal energy.
					}

					So(fluid.heat.Float32Slice()[particle], ShouldEqual, expectedHeat)
					So(fluid.vel.Float32Slice()[particle*3], ShouldEqual, 0)
					So(fluid.pos.Float32Slice()[particle*3], ShouldEqual, 0.5)
				}
			}
		})

		Convey("A populated field with negative internal energy still fails visibly", func() {
			for cell := range fluid.rho.Float32Slice() {
				fluid.rho.Float32Slice()[cell] = 1
				fluid.energy.Float32Slice()[cell] = -1
			}

			err := fluid.gatherParticles()
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "negative internal energy gathered")
			So(fluid.dbgHead.UInt32Slice()[0], ShouldEqual, 0)
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

func BenchmarkGasRK2(b *testing.B) {
	fluid, err := newWorkspace(64, 1, 1)

	if err != nil {
		b.Fatal(err)
	}

	defer fluid.Close()
	rho := fluid.rho.Float32Slice()
	energy := fluid.energy.Float32Slice()
	pulse := len(rho) / 2
	rho[pulse] = float32(5 * fluid.domain.RhoMin)
	energy[pulse] = rho[pulse] * 1e-5
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := fluid.gasRK2(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGatherParticles(b *testing.B) {
	fluid, err := newWorkspace(8, 8, 8)

	if err != nil {
		b.Fatal(err)
	}

	defer fluid.Close()
	// One particle per grid cell exercises both vacuum and populated gathers.
	fluid.particles = fluid.domain.CellCount()
	fluid.allocateParticles(fluid.particles)

	for cell := range fluid.particles {
		fluid.mass.Float32Slice()[cell] = 1
		fluid.pos.Float32Slice()[cell*3] = float32(cell%8) / 8
		fluid.pos.Float32Slice()[cell*3+1] = float32(cell/8%8) / 8
		fluid.pos.Float32Slice()[cell*3+2] = float32(cell/64) / 8
		fluid.rho.Float32Slice()[cell] = float32(cell % 2)
		fluid.energy.Float32Slice()[cell] = float32(cell % 2)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := fluid.gatherParticles(); err != nil {
			b.Fatal(err)
		}
	}
}
