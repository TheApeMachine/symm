package kraken

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestVenueErrors(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the body carries a Kraken error array", func() {
			errs := VenueErrors([]byte(`{"error":["EAPI:Rate limit exceeded"]}`))

			Convey("errors lift verbatim", func() {
				So(errs, ShouldResemble, []any{"EAPI:Rate limit exceeded"})
			})
		})

		Convey("When the body is empty", func() {
			errs := VenueErrors(nil)

			Convey("nil is returned", func() {
				So(errs, ShouldBeNil)
			})
		})
	})
}

func TestRateLimited(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the error is EAPI:Rate limit exceeded", func() {
			limited := RateLimited([]any{"EAPI:Rate limit exceeded"})

			Convey("RateLimited is true", func() {
				So(limited, ShouldBeTrue)
			})
		})

		Convey("When the error is invalid nonce", func() {
			limited := RateLimited("EAPI:Invalid nonce")

			Convey("RateLimited is false", func() {
				So(limited, ShouldBeFalse)
			})
		})

		Convey("When the error is throttled", func() {
			limited := RateLimited("EService:Throttled: 1700000000")

			Convey("RateLimited is false", func() {
				So(limited, ShouldBeFalse)
			})
		})
	})
}

func TestThrottleUntil(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When the error is EService: Throttled: unix", func() {
			until, ok := ThrottleUntil([]any{"EService: Throttled: 1700000000"})

			Convey("retry time is parsed in UTC", func() {
				So(ok, ShouldBeTrue)
				So(until.Unix(), ShouldEqual, int64(1700000000))
				So(until.Location(), ShouldEqual, time.UTC)
			})
		})

		Convey("When the error is a rate limit", func() {
			_, ok := ThrottleUntil("EAPI:Rate limit exceeded")

			Convey("ThrottleUntil is false", func() {
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When the throttle suffix is not a unix time", func() {
			_, ok := ThrottleUntil("EService: Throttled: nope")

			Convey("ThrottleUntil is false", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkVenueErrors(b *testing.B) {
	body := []byte(`{"error":["EAPI:Rate limit exceeded"]}`)

	b.ReportAllocs()

	for b.Loop() {
		_ = VenueErrors(body)
	}
}

func BenchmarkRateLimited(b *testing.B) {
	errs := []any{"EAPI:Rate limit exceeded"}

	b.ReportAllocs()

	for b.Loop() {
		_ = RateLimited(errs)
	}
}

func BenchmarkThrottleUntil(b *testing.B) {
	errs := []any{"EService: Throttled: 1700000000"}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = ThrottleUntil(errs)
	}
}
