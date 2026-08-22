package regulator

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestMarkAccumulator(t *testing.T) {
	Convey("Given mark feedbacks across an accounting interval", t, func() {
		var accumulator markAccumulator

		accumulator.observe(types.MarkFeedback{
			Symbol:        "BTC/USD",
			Mark:          100,
			PeakDrawdown:  -0.02,
			FloorDistance: 0.05,
			SurgeArmed:    true,
		}, 0, false)

		accumulator.observe(types.MarkFeedback{
			Symbol:        "BTC/USD",
			Mark:          102,
			PeakDrawdown:  -0.03,
			FloorDistance: 0.01,
			SurgeArmed:    false,
		}, math.Log(1.02), true)

		context := accumulator.snapshot()

		Convey("It should compute summary statistics across the observations", func() {
			So(context.samples, ShouldEqual, 2)
			So(context.returnSamples, ShouldEqual, 1)
			So(context.meanReturn, ShouldAlmostEqual, math.Log(1.02), 1e-12)
			So(context.worstDrawdown, ShouldAlmostEqual, -0.03, 1e-12)
			So(context.minimumFloor, ShouldAlmostEqual, 0.01, 1e-12)
			So(context.surgeFraction, ShouldAlmostEqual, 0.5, 1e-12)
		})

		accumulator.reset()

		Convey("Reset should clear all interval state", func() {
			empty := accumulator.snapshot()
			So(empty.samples, ShouldEqual, 0)
			So(empty.returnSamples, ShouldEqual, 0)
		})
	})
}

func TestHindsightAccumulator(t *testing.T) {
	Convey("Given hindsight attributions across an accounting interval", t, func() {
		var accumulator hindsightAccumulator

		accumulator.observe(types.HindsightFeedback{
			Symbol:         "BTC/USD",
			At:             time.Now(),
			Opportunity:    true,
			Captured:       true,
			RealizedReturn: 0.08,
		})

		accumulator.observe(types.HindsightFeedback{
			Symbol:         "SOL/USD",
			At:             time.Now(),
			Opportunity:    true,
			Captured:       true,
			RealizedReturn: -0.02,
		})

		accumulator.observe(types.HindsightFeedback{
			Symbol:          "ETH/USD",
			At:              time.Now(),
			Opportunity:     true,
			Missed:          true,
			MissedReturn:    0.10,
			DominantBlocker: "confidence",
		})

		accumulator.observe(types.HindsightFeedback{
			Symbol:          "AVAX/USD",
			At:              time.Now(),
			Opportunity:     true,
			Missed:          true,
			MissedReturn:    0.05,
			DominantBlocker: "contradiction",
		})

		context := accumulator.snapshot()

		Convey("It should compute capture ratio, false positive cost, and blocker fractions", func() {
			So(context.samples, ShouldEqual, 4)
			So(context.capturedSamples, ShouldEqual, 2)
			So(context.missedSamples, ShouldEqual, 2)
			So(context.meanCapturedReturn, ShouldAlmostEqual, 0.03, 1e-12)
			So(context.meanMissedReturn, ShouldAlmostEqual, 0.075, 1e-12)
			So(context.falsePositiveReturn, ShouldAlmostEqual, 0.02, 1e-12)
			So(context.opportunityPrevalence(), ShouldAlmostEqual, 1.0, 1e-12)

			// Total return = 2*0.03 + 2*0.075 = 0.06 + 0.15 = 0.21
			// Capture ratio = 0.06 / 0.21 = 2/7 ≈ 0.2857
			So(context.captureRatio(), ShouldAlmostEqual, 0.06/0.21, 1e-6)

			thesisF, confF, suppF, contF, graphF := context.blockerFractions()
			So(thesisF, ShouldEqual, 0.0)
			So(confF, ShouldAlmostEqual, 0.5, 1e-12)
			So(suppF, ShouldEqual, 0.0)
			So(contF, ShouldAlmostEqual, 0.5, 1e-12)
			So(graphF, ShouldEqual, 0.0)
		})
	})
}

func TestRegulatorContext(t *testing.T) {
	Convey("Given raw wallet, mark, and rich hindsight statistics", t, func() {
		marks := markContext{
			samples:       2,
			returnSamples: 1,
			meanReturn:    0.01,
			worstDrawdown: -0.02,
			minimumFloor:  0.03,
			surgeFraction: 0.5,
		}

		hindsight := hindsightContext{
			samples:              4,
			capturedSamples:      2,
			missedSamples:        2,
			meanCapturedReturn:   0.04,
			meanMissedReturn:     0.06,
			falsePositiveReturn:  0.01,
			confidenceBlockCount: 2,
		}

		vector := regulatorContext(0.05, -0.01, true, marks, hindsight)

		Convey("It should construct a 17-dimensional normalized feature vector", func() {
			So(vector, ShouldHaveLength, regulatorContextCount)
			So(vector[0], ShouldEqual, 0.05)   // periodReturn
			So(vector[1], ShouldEqual, -0.01)  // drawdown
			So(vector[2], ShouldEqual, 1.0)    // activeVal
			So(vector[3], ShouldEqual, 0.01)   // marks.meanReturn
			So(vector[4], ShouldEqual, -0.02)  // marks.worstDrawdown
			So(vector[5], ShouldEqual, 0.03)   // marks.minimumFloor
			So(vector[6], ShouldEqual, 0.5)    // marks.surgeFraction
			So(vector[7], ShouldEqual, 1.0)    // opportunityPrevalence
			So(vector[8], ShouldEqual, 0.04)   // meanCapturedReturn
			So(vector[9], ShouldEqual, 0.06)   // meanMissedReturn
			So(vector[10], ShouldAlmostEqual, 0.08/0.20, 1e-6) // captureRatio
			So(vector[11], ShouldEqual, 0.01)  // falsePositiveReturn
			So(vector[12], ShouldEqual, 0.0)   // thesisBlock
			So(vector[13], ShouldEqual, 1.0)   // confBlock (2/2 = 1.0)
			So(vector[14], ShouldEqual, 0.0)   // suppBlock
			So(vector[15], ShouldEqual, 0.0)   // contBlock
			So(vector[16], ShouldEqual, 0.0)   // graphBlock
		})
	})
}
