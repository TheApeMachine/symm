package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestInstrumentRulesCacheLoadFromAssetPairs(t *testing.T) {
	Convey("Given REST asset pair metadata", t, func() {
		cache := NewInstrumentRulesCache(t.Context())
		loaded := cache.LoadFromAssetPairs(market.AssetPairs{
			"XXBTZEUR": {
				Wsname:       "BTC/EUR",
				Base:         "XXBT",
				Quote:        "ZEUR",
				LotDecimals:  8,
				PairDecimals: 1,
				CostDecimals: 5,
				Ordermin:     "0.0001",
				Costmin:      "10",
				TickSize:     "0.1",
				Status:       "online",
			},
		})

		Convey("It should enforce Kraken minimums on entries", func() {
			So(loaded, ShouldEqual, 1)

			raisedQty, _, err := cache.PrepareEntryOrder(
				"BTC/EUR",
				0.00001,
				100_000,
				trading.Market,
			)

			So(err, ShouldBeNil)
			So(raisedQty, ShouldBeGreaterThan, 0.00001)

			rejectErr := cache.ValidateOrder("BTC/EUR", 0.00001, 50, trading.Market)

			So(rejectErr, ShouldNotBeNil)
		})
	})
}

func BenchmarkInstrumentRulesCacheLoadFromAssetPairs(b *testing.B) {
	pairs := market.AssetPairs{
		"XXBTZEUR": {
			Wsname:      "BTC/EUR",
			LotDecimals: 8,
			Ordermin:    "0.0001",
			Costmin:     "10",
			TickSize:    "0.1",
			Status:      "online",
		},
	}

	for b.Loop() {
		cache := NewInstrumentRulesCache(b.Context())
		_ = cache.LoadFromAssetPairs(pairs)
	}
}
