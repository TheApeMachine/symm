package logic

import (
	"testing"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPhysicalManifoldEvidence(testingTB *testing.T) {
	Convey("Given manifold readback features", testingTB, func() {
		physical := &physicalManifold{}
		reading := pmanifold.Reading{
			PressureGradNorm: 12,
			CoherenceMag2:    0.2,
			GuidanceSpeed:    0.5,
			ViscosityProxy:   0.5,
		}
		rho := rhoEvidence{
			mass:     10,
			peak:     3,
			entropy:  1.5,
			gradient: 2.5,
		}
		oscillators := oscillatorEvidence{
			coherence: 0.8,
			kinetic:   0.7,
			thermal:   0.6,
		}

		Convey("When physical evidence is assembled", func() {
			evidence, err := physical.evidence(
				reading,
				pmanifold.Reading{PressureGradNorm: 4, ViscosityProxy: 5},
				[][]float64{{1, 0}, {0, 3}},
				rho,
				oscillators,
			)

			Convey("Then it should preserve field state instead of classifying it", func() {
				So(err, ShouldBeNil)
				So(evidence.category, ShouldEqual, types.CategoryType("physical_field"))
				So(evidence.rho.mass, ShouldEqual, rho.mass)
				So(evidence.rho.gradient, ShouldEqual, rho.gradient)
				So(evidence.oscillators.coherence, ShouldEqual, oscillators.coherence)
				So(evidence.strength, ShouldEqual, rho.gradient)
				So(evidence.shock, ShouldEqual, 4)
				So(evidence.resistance, ShouldEqual, 5)
			})
		})
	})
}

func TestPhysicalManifoldCell(testingTB *testing.T) {
	Convey("Given a normalized clamp position", testingTB, func() {
		physical := &physicalManifold{}

		Convey("When it is mapped to solver cells", func() {
			Convey("Then the edge positions should map to grid edges", func() {
				So(physical.cell(0, 8), ShouldEqual, 0)
				So(physical.cell(1, 8), ShouldEqual, 7)
			})

			Convey("Then the midpoint should map to the nearest semantic middle cell", func() {
				So(physical.cell(0.5, 8), ShouldEqual, 4)
			})
		})
	})
}

func TestPhysicalFieldRho(testingTB *testing.T) {
	Convey("Given a rho projection", testingTB, func() {
		field := newPhysicalField()
		rows := [][]float64{
			{1, 0},
			{0, 3},
		}

		Convey("When rho evidence is measured", func() {
			rho, err := field.Rho(rows)

			Convey("Then it should expose field mass and shape", func() {
				So(err, ShouldBeNil)
				So(rho.mass, ShouldEqual, 4)
				So(rho.peak, ShouldEqual, 3)
				So(rho.entropy, ShouldBeGreaterThan, 0)
				So(rho.gradient, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestPhysicalFieldOscillators(testingTB *testing.T) {
	Convey("Given oscillator readback", testingTB, func() {
		field := newPhysicalField()
		oscillators := []pmanifold.Oscillator{
			{Amplitude: 1, Phase: 0, VelX: 1, Heat: 2, Omega: 3},
			{Amplitude: 1, Phase: 0, VelX: 2, Heat: 4, Omega: 5},
		}

		Convey("When oscillator evidence is measured", func() {
			evidence, err := field.Oscillators(oscillators)

			Convey("Then it should expose carrier coherence and energy", func() {
				So(err, ShouldBeNil)
				So(evidence.coherence, ShouldEqual, 1)
				So(evidence.kinetic, ShouldEqual, 1.5)
				So(evidence.thermal, ShouldEqual, 3)
				So(evidence.omega, ShouldEqual, 4)
			})
		})
	})
}
