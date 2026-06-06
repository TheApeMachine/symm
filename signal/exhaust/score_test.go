package exhaust

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/ring"
)

func TestExitScoreLong(t *testing.T) {
	Convey("Given deteriorating long-side book history", t, func() {
		history := symbolHistory{
			bidDepths:  ring.NewFloatRing(exitHistoryCap),
			spreads:    ring.NewFloatRing(exitHistoryCap),
			pressures:  ring.NewFloatRing(exitHistoryCap),
			imbalances: ring.NewFloatRing(exitHistoryCap),
			densities:  ring.NewFloatRing(exitHistoryCap),
		}

		for _, value := range []float64{10, 10, 10, 10, 8, 6} {
			history.bidDepths.Push(value)
			history.spreads.Push(4)
			history.pressures.Push(0.7)
			history.imbalances.Push(0.5)
			history.densities.Push(3)
		}

		history.bidDepths.Push(1)
		history.spreads.Push(15)
		history.pressures.Push(0.05)
		history.imbalances.Push(-0.6)

		urgency, category, evidence := exitScoreLong(history)

		Convey("It should score positive exit urgency", func() {
			So(urgency, ShouldBeGreaterThan, 0)
			So(urgency, ShouldBeLessThan, 1)
			So(category, ShouldNotEqual, types.CategoryTypeNone)
			So(evidence, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestExitScoreLongUsesForwardFeedbackOnComponents(t *testing.T) {
	Convey("Given learned exhaust feedback before scoring components", t, func() {
		types.ResetSourceFeedback()
		defer types.ResetSourceFeedback()

		history := symbolHistory{
			bidDepths:  ring.NewFloatRing(exitHistoryCap),
			spreads:    ring.NewFloatRing(exitHistoryCap),
			pressures:  ring.NewFloatRing(exitHistoryCap),
			imbalances: ring.NewFloatRing(exitHistoryCap),
			densities:  ring.NewFloatRing(exitHistoryCap),
		}

		for _, value := range []float64{10, 10, 10, 10, 8, 6} {
			history.bidDepths.Push(value)
			history.spreads.Push(4)
			history.pressures.Push(0.7)
			history.imbalances.Push(0.5)
			history.densities.Push(3)
		}

		history.bidDepths.Push(1)
		history.spreads.Push(15)
		history.pressures.Push(0.05)
		history.imbalances.Push(-0.6)

		rawUrgency, _, rawEvidence := exitScoreLong(history)
		_, feedbackErr := types.UpdateSourceFeedback(types.SourceExhaustion, 0.1, 2, 1)
		So(feedbackErr, ShouldBeNil)
		adjustedUrgency, _, adjustedEvidence := exitScoreLong(history)

		Convey("It should tune component values before urgency and evidence are derived", func() {
			So(adjustedUrgency, ShouldBeGreaterThan, rawUrgency)
			So(adjustedEvidence, ShouldBeGreaterThanOrEqualTo, rawEvidence)
		})
	})
}

func BenchmarkExitScoreLong(b *testing.B) {
	types.ResetSourceFeedback()
	defer types.ResetSourceFeedback()

	history := symbolHistory{
		bidDepths:  ring.NewFloatRing(exitHistoryCap),
		spreads:    ring.NewFloatRing(exitHistoryCap),
		pressures:  ring.NewFloatRing(exitHistoryCap),
		imbalances: ring.NewFloatRing(exitHistoryCap),
		densities:  ring.NewFloatRing(exitHistoryCap),
	}

	for _, value := range []float64{10, 10, 10, 10, 8, 6, 1} {
		history.bidDepths.Push(value)
		history.spreads.Push(4)
		history.pressures.Push(0.7)
		history.imbalances.Push(0.5)
		history.densities.Push(3)
	}

	history.spreads.Push(15)
	history.pressures.Push(0.05)
	history.imbalances.Push(-0.6)
	_, _ = types.UpdateSourceFeedback(types.SourceExhaustion, 0.1, 1.25, 1)

	for b.Loop() {
		_, _, _ = exitScoreLong(history)
	}
}
