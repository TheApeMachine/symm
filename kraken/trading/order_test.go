package trading

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestAddParamsJSONTags(t *testing.T) {
	Convey("Given order params", t, func() {
		params := AddParams{
			OrderType:  Limit,
			Side:       Buy,
			Symbol:     "BTC/EUR",
			OrderQty:   0.01,
			LimitPrice: 50_000,
			ClOrdID:    "test-order",
		}

		Convey("It should retain side and type constants", func() {
			So(string(params.OrderType), ShouldEqual, "limit")
			So(string(params.Side), ShouldEqual, "buy")
			So(params.ClOrdID, ShouldEqual, "test-order")
		})
	})
}

func TestEntryTransitTTL(t *testing.T) {
	t.Cleanup(viper.Reset)

	Convey("Given no configured transit ttl", t, func() {
		Convey("It should default to five seconds", func() {
			So(EntryTransitTTL(), ShouldEqual, 5*time.Second)
		})
	})

	Convey("Given a configured transit ttl", t, func() {
		viper.Set("trading.entry.transit_ttl", 12*time.Second)

		Convey("It should return the configured duration", func() {
			So(EntryTransitTTL(), ShouldEqual, 12*time.Second)
		})
	})
}

func BenchmarkEntryTransitTTL(b *testing.B) {
	for b.Loop() {
		_ = EntryTransitTTL()
	}
}
