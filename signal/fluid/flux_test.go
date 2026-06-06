package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluxAccumulatorRoll(t *testing.T) {
	Convey("Given a flux accumulator with a volume target", t, func() {
		accumulator := newFluxAccumulator()
		So(accumulator.setTarget(10), ShouldBeNil)

		So(accumulator.addTrade(6), ShouldBeNil)
		So(accumulator.addBook(4), ShouldBeNil)
		So(accumulator.addTrade(4), ShouldBeNil)

		Convey("It should close a bar once target volume trades", func() {
			bookFlux, tradeFlux, err := accumulator.completedBar()

			So(err, ShouldBeNil)
			So(tradeFlux, ShouldEqual, 10)
			So(bookFlux, ShouldEqual, 4)
		})
	})
}

func TestFluxAccumulatorRejectsUnsetTarget(t *testing.T) {
	Convey("Given a flux accumulator without a volume target", t, func() {
		accumulator := newFluxAccumulator()

		Convey("It should reject trade folds", func() {
			So(accumulator.addTrade(1), ShouldNotBeNil)
		})

		Convey("It should reject book folds without a target", func() {
			So(accumulator.addBook(1), ShouldNotBeNil)
		})
	})
}

func TestFluxAccumulatorAccumulatesBookBeforeTradeVolume(t *testing.T) {
	Convey("Given book churn before the bar has trade volume", t, func() {
		accumulator := newFluxAccumulator()
		So(accumulator.setTarget(10), ShouldBeNil)
		So(accumulator.addBook(5), ShouldBeNil)
		So(accumulator.addTrade(10), ShouldBeNil)

		Convey("It should include pre-trade book churn in the closed bar", func() {
			bookFlux, tradeFlux, err := accumulator.completedBar()

			So(err, ShouldBeNil)
			So(tradeFlux, ShouldEqual, 10)
			So(bookFlux, ShouldEqual, 5)
		})
	})
}

func BenchmarkFluxAccumulatorAddTrade(b *testing.B) {
	accumulator := newFluxAccumulator()
	_ = accumulator.setTarget(10)

	for b.Loop() {
		_ = accumulator.addTrade(1)
	}
}
