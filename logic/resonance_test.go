package logic

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/strategy"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResonancePrice(testingTB *testing.T) {
	Convey("Given a thesis with price evidence", testingTB, func() {
		thesis := strategy.NewThesis()
		at := time.Unix(10, 0)
		thesis.AddEvidence("price", 100.0)
		thesis.AddEvidence("price_at", at)

		Convey("When the resonance price is read", func() {
			price, priceAt, ok := resonancePrice(thesis)

			Convey("Then the price and event time should both be required", func() {
				So(ok, ShouldBeTrue)
				So(price, ShouldEqual, 100.0)
				So(priceAt, ShouldEqual, at)
			})
		})
	})

	Convey("Given a thesis without a price timestamp", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", 100.0)

		Convey("When the resonance price is read", func() {
			_, _, ok := resonancePrice(thesis)

			Convey("Then the price should not be usable for horizon training", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}
