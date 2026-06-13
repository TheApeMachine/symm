package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHoldingsIsHolding(t *testing.T) {
	Convey("Given a symbol inventory snapshot", t, func() {
		holdings := NewHoldings()
		holdings.SetQuantity("BTC/USD", 0.25)

		Convey("It should report holding when quantity is positive", func() {
			So(holdings.IsHolding("BTC/USD"), ShouldBeTrue)
			So(holdings.IsHolding("ETH/USD"), ShouldBeFalse)
		})
	})
}

func TestSubjectHoldingEvaluate(t *testing.T) {
	Convey("Given a holding subject and measurement symbol", t, func() {
		holdings := NewHoldings()
		holdings.SetQuantity("BTC/USD", 1)

		subject := NewSubject(
			SourceNone,
			SubjectHolding,
			nil,
			nil,
			nil,
			0,
			0,
			0,
			0,
			0,
			0,
			0,
		)
		subject.Holding = &HoldingSubject{Held: true}

		measurement := NewMeasurement(
			SourceHawkes,
			"BTC/USD",
			0,
			0,
			0,
			0,
			0,
			CategoryFrenzy,
			RegimeTypeNone,
			PositionTypeNone,
			0,
			0,
		)

		Convey("It should match held inventory", func() {
			match, err := subject.Evaluate(measurement, holdings)

			So(err, ShouldBeNil)
			So(match, ShouldBeTrue)
		})

		Convey("It should reject not-holding checks when inventory exists", func() {
			subject.Holding.Held = false

			match, err := subject.Evaluate(measurement, holdings)

			So(err, ShouldBeNil)
			So(match, ShouldBeFalse)
		})

		Convey("It should reject held checks when inventory is absent", func() {
			subject.Holding.Held = true
			emptyHoldings := NewHoldings()

			match, err := subject.Evaluate(measurement, emptyHoldings)

			So(err, ShouldBeNil)
			So(match, ShouldBeFalse)
		})

		Convey("It should rank open positions by entry confidence", func() {
			holdings.SetPosition("BTC/USD", 1, 0.9, false)
			holdings.SetPosition("ETH/USD", 1, 0.7, false)

			So(holdings.StrictlyHigherConfidenceCount(0.75), ShouldEqual, 1)
			So(holdings.StrictlyHigherConfidenceCount(0.95), ShouldEqual, 0)
		})

		Convey("It should error when holdings are missing", func() {
			match, err := subject.Evaluate(measurement, nil)

			So(err, ShouldNotBeNil)
			So(match, ShouldBeFalse)
		})
	})
}
