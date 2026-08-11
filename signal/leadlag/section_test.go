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

func TestFeatures(t *testing.T) {
	Convey("Given overlapping anchor and follower price histories", t, func() {
		section := NewSection()
		start := time.Unix(1_700_000_000, 0).UTC()
		section.SetAnchor("BTC/USD")

		for index, price := range []float64{100, 101, 103} {
			at := start.Add(time.Duration(index) * time.Second)
			So(section.ObservePrice("BTC/USD", price, at), ShouldBeTrue)
		}

		for index, price := range []float64{50, 50.5, 51} {
			at := start.Add(time.Second + time.Duration(index)*time.Second)
			So(section.ObservePrice("ALT/USD", price, at), ShouldBeTrue)
		}

		features := section.Features("ALT/USD")
		anchorFeatures := section.Features("BTC/USD")

		Convey("It should retain the actual common observation interval", func() {
			So(features.ObservedFrom, ShouldEqual, start.Add(time.Second))
			So(features.ObservedAt, ShouldEqual, start.Add(3*time.Second))
			So(features.PeerPrice, ShouldEqual, 103.0)
			So(features.PeerFrom, ShouldEqual, start)
			So(features.PeerAt, ShouldEqual, start.Add(2*time.Second))
			So(anchorFeatures.ObservedFrom, ShouldEqual, start)
			So(anchorFeatures.ObservedAt, ShouldEqual, start.Add(2*time.Second))
		})
	})
}
