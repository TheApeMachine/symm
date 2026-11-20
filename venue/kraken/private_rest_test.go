package kraken

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
)

func TestParseRESTTier(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the tier is starter", func() {
			tier := ParseRESTTier("starter")

			Convey("max and decay match the REST docs", func() {
				So(tier.Max, ShouldEqual, 15.0)
				So(tier.Decay, ShouldEqual, 0.33)
			})
		})

		Convey("When the tier is pro", func() {
			tier := ParseRESTTier("pro")

			Convey("max and decay match the REST docs", func() {
				So(tier.Max, ShouldEqual, 20.0)
				So(tier.Decay, ShouldEqual, 1.0)
			})
		})

		Convey("When the tier is unknown", func() {
			tier := ParseRESTTier("")

			Convey("intermediate is the default", func() {
				So(tier.Name, ShouldEqual, TierIntermediate)
				So(tier.Max, ShouldEqual, 20.0)
				So(tier.Decay, ShouldEqual, 0.5)
			})
		})
	})
}

func TestPrivateRESTCost(t *testing.T) {
	Convey("Setup", t, func() {
		rest := NewPrivateREST()

		Convey("When the url is TradesHistory", func() {
			Convey("cost is +2", func() {
				So(rest.Cost("https://api.kraken.com/0/private/TradesHistory"), ShouldEqual, 2.0)
			})
		})

		Convey("When the url is Balance", func() {
			Convey("cost is +1", func() {
				So(rest.Cost("https://api.kraken.com/0/private/Balance"), ShouldEqual, 1.0)
			})
		})
	})
}

func TestPrivateRESTHoldCommit(t *testing.T) {
	Convey("Setup", t, func() {
		viper.Set("kraken.verification_tier", TierIntermediate)
		clock := time.Unix(1_700_000_000, 0)
		rest := NewPrivateREST()
		rest.now = func() time.Time { return clock }

		Convey("When commits fill the intermediate max", func() {
			for i := 0; i < 20; i++ {
				So(rest.Hold(t.Context(), 1), ShouldBeNil)
				rest.Commit(1)
				rest.Release()
			}

			Convey("Waiting reports decay headroom for the next call", func() {
				So(rest.Waiting(), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When rate limited is observed", func() {
			So(rest.Hold(t.Context(), 1), ShouldBeNil)
			rest.Commit(1)
			rest.ObserveError(errors.New("EAPI:Rate limit exceeded"))
			rest.Release()

			Convey("counter snaps to max and Waiting is positive", func() {
				So(rest.Waiting(), ShouldBeGreaterThan, 0)
			})
		})

		Convey("When throttled to a future unix time", func() {
			until := clock.Add(30 * time.Second)
			So(rest.Hold(t.Context(), 1), ShouldBeNil)
			rest.Observe("EService: Throttled: " + strconv.FormatInt(until.Unix(), 10))
			rest.Release()

			Convey("Waiting respects the throttle deadline", func() {
				So(rest.Waiting(), ShouldAlmostEqual, 30*time.Second, time.Millisecond)
			})
		})

		Convey("When Hold is cancelled", func() {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			err := rest.Hold(ctx, 1)

			Convey("the error is returned without locking forever", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func BenchmarkParseRESTTier(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = ParseRESTTier("intermediate")
	}
}

func BenchmarkPrivateRESTCost(b *testing.B) {
	rest := NewPrivateREST()

	b.ReportAllocs()

	for b.Loop() {
		_ = rest.Cost("https://api.kraken.com/0/private/TradesHistory")
	}
}
