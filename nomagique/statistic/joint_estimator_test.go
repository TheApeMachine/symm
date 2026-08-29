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

			// With event-time decay alpha=0.5 (elapsed == cadence), the
			// weight moments are sumW=1.0, sumW2=0.5, so N_eff=2.
			neff, hasNeff := second.Get(estimator.Neff())
			So(hasNeff, ShouldBeTrue)
			So(neff, ShouldAlmostEqual, 2.0, 1e-6)
		})
	})
}

/*
TestJointDecayedEstimatorKillFixture is the covariance re-centering kill
fixture: previous mu=0, C=0, a single new observation x=2 with alpha=.5. The
weighted observations [0,2] have mean 1 and variance 1; the covariance around
the UPDATED mean must therefore be 1, not 2. The naive C' = (1-a)C + a·r·rᵀ
reports 2; the center-corrected C' = (1-a)C + a(1-a)·r·rᵀ reports 1.
*/
func TestJointDecayedEstimatorKillFixture(t *testing.T) {
	Convey("Given mu=0, C=0 and one new observation x=2 at alpha=0.5", t, func() {
		estimator := NewJointDecayedEstimator("jt3", 3)
		in := []types.Symbol{types.MustIntern("jv0"), types.MustIntern("jv1"), types.MustIntern("jv2")}
		// Fixed alpha=0.5 so the elapsed-time cadence seeding does not interfere.
		pipeline := estimator.Primitive(in, func(*types.Frame) float64 { return 0.5 })
		stream := types.NewStream(pipeline, types.Frame{})

		stream.Step(jointFrame([3]float64{0, 0, 0}, 1000, 0))
		second := stream.Step(jointFrame([3]float64{2, 0, 0}, 1001, 0))

		Convey("the covariance around the updated mean is 1, not 2", func() {
			covariance, hasCov := second.Get(estimator.cov[0][0])
			So(hasCov, ShouldBeTrue)
			So(covariance, ShouldAlmostEqual, 1.0, 1e-6)

			mean, hasMean := second.Get(estimator.mu[0])
			So(hasMean, ShouldBeTrue)
			So(mean, ShouldAlmostEqual, 1.0, 1e-6)
		})
	})
}

/*
TestJointDecayedEstimatorZeroElapsedAlpha asserts event-time decay gives alpha=0
at zero elapsed: a simultaneous observation must contribute zero weight, never
replace the committed state with alpha=1.
*/
func TestJointDecayedEstimatorZeroElapsedAlpha(t *testing.T) {
	Convey("Given a prior observation and a new observation at the same timestamp", t, func() {
		estimator := NewJointDecayedEstimator("jt4", 3)
		in := []types.Symbol{types.MustIntern("jv0"), types.MustIntern("jv1"), types.MustIntern("jv2")}
		pipeline := estimator.Primitive(in, nil)
		stream := types.NewStream(pipeline, types.Frame{})

		stream.Step(jointFrame([3]float64{0, 0, 0}, 1000, 0))
		output := stream.Step(jointFrame([3]float64{5, 5, 5}, 1000, 0))

		Convey("the simultaneous observation does not replace the committed mean", func() {
			mean, hasMean := output.Get(estimator.mu[0])
			So(hasMean, ShouldBeTrue)
			So(mean, ShouldEqual, 0.0)
		})
	})
}

/*
TestJointDecayedEstimatorReadinessBecomesDefined asserts the joint SNR can
actually become ready once the pre-observation effective support N_eff exceeds
the dimension count. With a fixed alpha=0.2 the effective memory grows past 3,
so readiness is reachable — the gate uses N_eff, not the normalized weight mass
which saturates at 1 and could never exceed 3.
*/
func TestJointDecayedEstimatorReadinessBecomesDefined(t *testing.T) {
	Convey("Given a fixed decay whose N_eff exceeds the dimension count", t, func() {
		estimator := NewJointDecayedEstimator("jt5", 3)
		in := []types.Symbol{types.MustIntern("jv0"), types.MustIntern("jv1"), types.MustIntern("jv2")}
		pipeline := estimator.Primitive(in, func(*types.Frame) float64 { return 0.2 })
		stream := types.NewStream(pipeline, types.Frame{})

		// Three linearly independent residual directions, then a fourth that
		// resolves a non-degenerate covariance. alpha=0.2 → N_eff → 9 > 3.
		observations := [][3]float64{
			{1, 0, 0},
			{2, 1, 0},
			{2, 2, 1},
			{3, 2, 1},
			{3, 3, 2},
			{4, 3, 2},
		}

		var last types.Frame

		for index, observation := range observations {
			last = stream.Step(jointFrame(observation, 1000+float64(index), 0))
		}

		Convey("the joint SNR becomes defined with a ready marker of 1", func() {
			ready, hasReady := last.Get(estimator.JointReady())
			So(hasReady, ShouldBeTrue)
			So(ready, ShouldEqual, 1.0)

			_, hasSNR := last.Get(estimator.SNR())
			So(hasSNR, ShouldBeTrue)
		})
	})
}
