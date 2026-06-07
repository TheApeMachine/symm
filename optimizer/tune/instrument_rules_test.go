package tune

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/optimizer/replay"
)

func TestAttachInstrumentRules(t *testing.T) {
	Convey("Given injected instrument rules", t, func() {
		cache := broker.NewInstrumentRulesCache(t.Context())
		cache.LoadFromAssetPairs(market.AssetPairs{
			"XXBTZEUR": {
				Wsname:      "BTC/EUR",
				LotDecimals: 8,
				Ordermin:    "0.0001",
				Costmin:     "10",
				TickSize:    "0.1",
				Status:      "online",
			},
		})
		costs := replay.DefaultReplayCosts()

		err := attachInstrumentRules(context.Background(), &costs, cache)

		Convey("It should wire replay costs without REST", func() {
			So(err, ShouldBeNil)
			So(costs.InstrumentRules, ShouldEqual, cache)
		})
	})

	Convey("Given instrument rule loading disabled", t, func() {
		viper.Set("optimizer.tune.load_instrument_rules", false)
		defer viper.Set("optimizer.tune.load_instrument_rules", true)

		costs := replay.DefaultReplayCosts()

		err := attachInstrumentRules(context.Background(), &costs, nil)

		Convey("It should leave replay costs unset", func() {
			So(err, ShouldBeNil)
			So(costs.InstrumentRules, ShouldBeNil)
		})
	})
}
