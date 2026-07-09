package logic

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/strategy"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResonancePrice(testingTB *testing.T) {
	Convey("Given resonance price evidence", testingTB, func() {
		thesis := strategy.NewThesis()
		at := time.Unix(10, 0)
		thesis.AddEvidence("price", 100.0)
		thesis.AddEvidence("price_at", at)
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			price, priceAt, ok := resonance.Price()

			Convey("Then the usable price and event time are returned", func() {
				So(ok, ShouldBeTrue)
				So(price, ShouldEqual, 100.0)
				So(priceAt, ShouldEqual, at)
			})
		})
	})

	Convey("Given resonance evidence without price", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price_at", time.Unix(10, 0))
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a non-float price", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", "100")
		thesis.AddEvidence("price_at", time.Unix(10, 0))
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a non-positive price", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", 0.0)
		thesis.AddEvidence("price_at", time.Unix(10, 0))
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a non-finite price", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", math.NaN())
		thesis.AddEvidence("price_at", time.Unix(10, 0))
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence without a price timestamp", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", 100.0)
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a non-time timestamp", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", 100.0)
		thesis.AddEvidence("price_at", "2026-07-09")
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})

	Convey("Given resonance evidence with a zero timestamp", testingTB, func() {
		thesis := strategy.NewThesis()
		thesis.AddEvidence("price", 100.0)
		thesis.AddEvidence("price_at", time.Time{})
		resonance := &Resonance{thesis: thesis}

		Convey("When the resonance price is read", func() {
			_, _, ok := resonance.Price()

			Convey("Then it is not usable", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}
