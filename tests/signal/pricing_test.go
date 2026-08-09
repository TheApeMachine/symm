package signal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestGeneratorNewGeneratorFromSymbol(t *testing.T) {
	Convey("Given explicit per-symbol venue characteristics", t, func() {
		symbol := testtypes.NewSymbol("PROFILE/USD", 100, 31)
		symbol.QuantityPrecision = 4
		symbol.BaseSpreadFraction = 0.001
		symbol.BookDepthLevels = 4
		symbol.DepthQuantityScale = 1.25
		symbol.FactorLoading = -0.5
		generator := NewGeneratorFromSymbol(symbol)

		Convey("The generator should preserve every market-facing parameter", func() {
			So(generator.quantityScale, ShouldEqual, 10_000.0)
			So(generator.spreadFraction, ShouldEqual, 0.001)
			So(generator.depthLevels, ShouldEqual, 4)
			So(generator.depthScale, ShouldEqual, 1.25)
			So(generator.factorLoading, ShouldEqual, -0.5)
		})
	})
}

func TestGeneratorSetTime(t *testing.T) {
	Convey("Given an explicit UTC replay anchor", t, func() {
		generator := NewGenerator("TIME/USD", 100, 0.01, 2, 32)
		start := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
		So(generator.SetTime(start), ShouldBeNil)
		sample := generator.Step()

		Convey("The first event should advance from that anchor by its cadence", func() {
			So(sample.Timestamp, ShouldEqual, start.Add(100*time.Millisecond))
			So(generator.SetTime(time.Time{}), ShouldNotBeNil)
		})
	})
}

func TestGeneratorConfigureDepth(t *testing.T) {
	Convey("Given a valid finite-depth configuration", t, func() {
		generator := NewGenerator("DEPTH/USD", 100, 0.01, 2, 33)
		So(generator.ConfigureDepth(3, 1.5), ShouldBeNil)

		Convey("The next sample should expose every configured level", func() {
			So(generator.Step().Bids, ShouldHaveLength, 3)
			So(generator.ConfigureDepth(0, 1), ShouldNotBeNil)
		})
	})
}

func TestGeneratorConfigureProfiles(t *testing.T) {
	Convey("Given a detached scenario regime contract", t, func() {
		generator := NewGenerator("PROFILE/USD", 100, 0.01, 2, 34)
		profiles := testtypes.CloneProfiles(testtypes.DefaultProfiles)
		So(generator.ConfigureProfiles(profiles), ShouldBeNil)
		profile := profiles[testtypes.FastPump]
		profile.Drift = 99
		profiles[testtypes.FastPump] = profile

		Convey("Caller mutation should not alter the installed profile", func() {
			So(generator.profiles[testtypes.FastPump].Drift, ShouldNotEqual, 99.0)
			So(generator.ConfigureProfiles(nil), ShouldNotBeNil)
		})
	})
}
