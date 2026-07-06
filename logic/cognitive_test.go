package logic

import (
	"testing"

	"github.com/spf13/viper"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPhysicalVector(testingTB *testing.T) {
	Convey("Given physical manifold readings", testingTB, func() {
		physical := physicalEvidence{
			reading: pmanifold.Reading{
				PressureGradX:    0.1,
				PressureGradY:    0.2,
				PressureGradZ:    0.3,
				PressureGradNorm: 0.4,
				Divergence:       0.5,
				CoherenceMag2:    0.6,
				GuidanceSpeed:    0.7,
				ViscosityProxy:   0.8,
			},
			projection: pmanifold.Reading{
				PressureGradNorm: 0.9,
				Divergence:       1.0,
				GuidanceSpeed:    1.1,
				ViscosityProxy:   1.2,
			},
			rho: rhoEvidence{
				mass:     1.3,
				peak:     1.4,
				entropy:  1.5,
				gradient: 1.6,
			},
			oscillators: oscillatorEvidence{
				coherence: 1.7,
				kinetic:   1.8,
				thermal:   1.9,
			},
		}

		Convey("When the resonance sensory vector is built", func() {
			vector := physicalVector(physical)

			Convey("Then it should contain twelve physical channels", func() {
				So(vector, ShouldHaveLength, 12)
				So(vector[0], ShouldEqual, 0.4)
				So(vector[5], ShouldEqual, 0.9)
				So(vector[8], ShouldEqual, 1.5)
				So(vector[11], ShouldEqual, 1.8)
			})
		})
	})
}

func TestCognitiveManifoldLearningPolicy(testingTB *testing.T) {
	Convey("Given explicit resonance learning config", testingTB, func() {
		restoreDecisionConfig()
		viper.Set("logic.resonance.learn", true)

		Convey("When the cognitive manifold is created", func() {
			cognitive, err := newCognitiveManifold()
			if cognitive != nil {
				defer cognitive.Close()
			}

			Convey("Then learning should follow the config", func() {
				So(err, ShouldBeNil)
				So(cognitive.learn, ShouldBeTrue)
			})
		})
	})
}
