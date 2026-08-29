package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func jointFrame(values [3]float64, sec, nsec float64) types.Frame {
	frame := types.Frame{}
	frame.Put(types.MustIntern("jv0"), values[0])
	frame.Put(types.MustIntern("jv1"), values[1])
	frame.Put(types.MustIntern("jv2"), values[2])
	frame.Put(types.EventTimeSec, sec)
	frame.Put(types.EventTimeNsec, nsec)

	return frame
}

func TestJointDecayedEstimatorReadiness(t *testing.T) {
	Convey("Given a 3-dimensional joint estimator with diagonal-ish history", t, func() {
		estimator := NewJointDecayedEstimator("jt", 3)
		in := []types.Symbol{types.MustIntern("jv0"), types.MustIntern("jv1"), types.MustIntern("jv2")}
		pipeline := estimator.Primitive(in, nil)
		stream := types.NewStream(pipeline, types.Frame{})

		Convey("the joint SNR is undefined until the covariance is estimable", func() {
			// Two observations: covariance rank is insufficient/invertibility
			// not yet reachable, so ready stays 0 and no SNR slot is written.
			stream.Step(jointFrame([3]float64{0, 0, 0}, 1000, 0))
			stream.Step(jointFrame([3]float64{0.1, 0.2, 0.0}, 1001, 0))
			third := stream.Step(jointFrame([3]float64{-0.1, 0.1, 0.2}, 1002, 0))

			ready, _ := third.Get(estimator.JointReady())
			So(ready, ShouldEqual, 0)

			_, hasSNR := third.Get(estimator.SNR())
			So(hasSNR, ShouldBeFalse)
		})
	})

	Convey("Given a diagonal covariance with known variance", t, func() {
		estimator := NewJointDecayedEstimator("jt2", 3)
		in := []types.Symbol{types.MustIntern("jv0"), types.MustIntern("jv1"), types.MustIntern("jv2")}
		pipeline := estimator.Primitive(in, nil)
		stream := types.NewStream(pipeline, types.Frame{})

		Convey("the effective support follows N_eff = (sum w)^2 / sum(w^2)", func() {
			stream.Step(jointFrame([3]float64{0, 0, 0}, 1000, 0))
			second := stream.Step(jointFrame([3]float64{0.5, 0.5, 0.5}, 1001, 0))

			// With event-time decay alpha=0.5 (elapsed == seeded cadence), the
			// weight moments are sumW=1.0, sumW2=0.5, so N_eff=2.
			neff, hasNeff := second.Get(estimator.Neff())
			So(hasNeff, ShouldBeTrue)
			So(neff, ShouldAlmostEqual, 2.0, 1e-6)
		})
	})
}
