package leadlag

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestObservePrice(t *testing.T) {
	Convey("Given a cached price observation", t, func() {
		section := NewSection()
		at := time.Unix(1_700_000_000, 0).UTC()

		So(section.ObservePrice("AAA/USD", 100, at), ShouldBeTrue)

		Convey("It should reject duplicate and regressed timestamps", func() {
			So(section.ObservePrice("AAA/USD", 101, at), ShouldBeFalse)
			So(section.ObservePrice("AAA/USD", 99, at.Add(-time.Second)), ShouldBeFalse)
			So(section.PriceSampleCount("AAA/USD"), ShouldEqual, 1)
		})
	})
}

func TestCausalAnchor(t *testing.T) {
	Convey("Given two completed cohort legs", t, func() {
		section := NewSection()
		start := time.Unix(1_700_000_000, 0).UTC()

		for _, sample := range []struct {
			symbol string
			first  float64
			second float64
		}{
			{symbol: "AAA/USD", first: 100, second: 110},
			{symbol: "BBB/USD", first: 100, second: 101},
			{symbol: "CCC/USD", first: 100, second: 99},
		} {
			So(section.ObservePrice(sample.symbol, sample.first, start), ShouldBeTrue)
			So(section.ObservePrice(sample.symbol, sample.second, start.Add(time.Second)), ShouldBeTrue)
		}

		Convey("It should select the prior leg's robust leader", func() {
			So(section.CausalAnchor(), ShouldEqual, "AAA/USD")
		})
	})
}
