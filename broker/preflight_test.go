package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestPreflightGates(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a complete fresh quote", t, func() {
		quote := Quote{
			Symbol:    "BTC/EUR",
			Bid:       99.95,
			Ask:       100,
			Last:      99.975,
			UpdatedAt: time.Now().UTC(),
		}

		Convey("It should accept maker limits", func() {
			So(PreflightGates(quote, trading.Buy, 0.01, trading.Limit), ShouldBeNil)
		})

		Convey("It should reject incomplete quotes", func() {
			incomplete := Quote{Symbol: "BTC/EUR", Last: 100, UpdatedAt: time.Now().UTC()}

			So(
				PreflightGates(incomplete, trading.Buy, 0.01, trading.Market),
				ShouldNotBeNil,
			)
		})

		Convey("It should reject stale quotes", func() {
			stale := quote
			stale.UpdatedAt = time.Now().UTC().Add(-1 * time.Hour)

			So(
				PreflightGates(stale, trading.Buy, 0.01, trading.Market),
				ShouldNotBeNil,
			)
		})
	})
}

func BenchmarkPreflightGates(b *testing.B) {
	testconfig.MustLoad()

	quote := Quote{
		Symbol:    "BTC/EUR",
		Bid:       99.95,
		Ask:       100,
		Last:      99.975,
		UpdatedAt: time.Now().UTC(),
	}

	for b.Loop() {
		_ = PreflightGates(quote, trading.Buy, 0.01, trading.Limit)
	}
}
