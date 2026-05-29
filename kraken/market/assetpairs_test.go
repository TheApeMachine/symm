package market

import (
	"encoding/json"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

const assetPairsFixture = `{
	"error": [],
	"result": {
		"0GEUR": {
			"altname": "0GEUR",
			"wsname": "0G/EUR",
			"aclass_base": "currency",
			"base": "0G",
			"aclass_quote": "currency",
			"quote": "ZEUR",
			"lot": "unit5",
			"cost_decimals": 5,
			"pair_decimals": 3,
			"lot_decimals": 5,
			"lot_multiplier": 1,
			"leverage_buy": [],
			"leverage_sell": [],
			"fees": [[0, 0.4]],
			"fees_maker": [[0, 0.25]],
			"fee_volume_currency": "ZEUR",
			"margin_call": 80,
			"margin_stop": 40,
			"ordermin": "5",
			"costmin": "0.45",
			"tick_size": "0.001",
			"status": "online",
			"execution_venue": "international",
			"long_position_limit": 0,
			"short_position_limit": 0
		}
	}
}`

func TestNewAssetPairs(t *testing.T) {
	convey.Convey("Given a Kraken asset-pairs payload", t, func() {
		pairs := AssetPairs{}

		convey.Convey("It should unmarshal pair metadata by internal name", func() {
			convey.So(json.Unmarshal([]byte(assetPairsFixture), &public.Response{
				Result: &pairs,
			}), convey.ShouldBeNil)
			convey.So(len(pairs), convey.ShouldEqual, 1)

			pair, ok := pairs["0GEUR"]

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(pair.Wsname, convey.ShouldEqual, "0G/EUR")
			convey.So(pair.Quote, convey.ShouldEqual, "ZEUR")
			convey.So(pair.TakerFeePctOr(0, 0.26), convey.ShouldEqual, 0.4)
		})
	})
}

func BenchmarkNewAssetPairs(b *testing.B) {
	payload := []byte(assetPairsFixture)

	for b.Loop() {
		pairs := AssetPairs{}
		_ = json.Unmarshal(payload, &public.Response{Result: &pairs})
	}
}
