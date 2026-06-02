package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFluxAccumulatorRoll(t *testing.T) {
	Convey("Given a flux accumulator", t, func() {
		accumulator := newFluxAccumulator(time.Minute)
		accumulator.setTarget(10)
		now := time.Now()

		accumulator.addBook(now, 4)
		accumulator.addTrade(now.Add(time.Second), 6)
		accumulator.addTrade(now.Add(2*time.Second), 4)

		Convey("It should close a bar once target volume trades", func() {
			So(accumulator.haveClosed, ShouldBeTrue)
			So(accumulator.tradeFlux(), ShouldEqual, 10)
			So(accumulator.bookFlux(), ShouldEqual, 4)
		})
	})
}

func BenchmarkFluxAccumulatorAddTrade(b *testing.B) {
	accumulator := newFluxAccumulator(time.Minute)
	now := time.Now()

	for b.Loop() {
		accumulator.addTrade(now, 1)
	}
}
