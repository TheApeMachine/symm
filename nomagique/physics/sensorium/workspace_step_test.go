package sensorium

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAdmitConserved(t *testing.T) {
	Convey("Given a vacuum-adjacent cell with leftover momentum", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() {
			fluid.Close()
		})
		rho := fluid.rho.Float32Slice()
		mom := fluid.mom.Float32Slice()
		energy := fluid.energy.Float32Slice()
		rho[0] = float32(fluid.domain.RhoMin) / 2
		mom[0] = 1
		energy[0] = 1
		rho[1] = float32(fluid.domain.RhoMin) * 2
		mom[3] = 1
		energy[1] = 1
		fluid.admitConserved()

		Convey("It should restore the kernel's vacuum triple and leave resolved cells alone", func() {
			So(mom[0], ShouldEqual, float32(0))
			So(energy[0], ShouldEqual, float32(0))
			So(mom[3], ShouldEqual, float32(1))
			So(energy[1], ShouldEqual, float32(1))
		})
	})
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
	Convey("Given one unit-mass oscillator at a cell centre", t, func() {
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
		fluid.mass.Float32Slice()[0] = 1
		fluid.phase.Float32Slice()[0] = 0
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

		Convey("CIC should deposit the oscillator phasor on the spatial grid", func() {
			So(float64(total), ShouldAlmostEqual, 1.0, 1e-5)
			So(peak, ShouldEqual, float32(1))
			So(peakCell, ShouldEqual, 4+8*(4+8*4))
		})
	})
}

func TestSplatParticleWave(t *testing.T) {
	Convey("Given one unit-amplitude oscillator at a cell centre", t, func() {
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
		fluid.mass.Float32Slice()[0] = 1
		fluid.phase.Float32Slice()[0] = 0
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

		Convey("CIC should deposit the phasor on the spatial grid", func() {
			So(float64(total), ShouldAlmostEqual, 1.0, 1e-5)
			So(peak, ShouldEqual, float32(1))
			So(peakCell, ShouldEqual, 4+8*(4+8*4))
			So(fluid.psiIm.Float32Slice()[peakCell], ShouldEqual, float32(0))
		})
	})
}
