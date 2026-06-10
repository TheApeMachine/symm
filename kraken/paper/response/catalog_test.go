package response

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

func testPairCatalog(t *testing.T) *PairCatalog {
	t.Helper()

	catalog, err := NewPairCatalog(market.AssetPairs{
		"XXBTZUSD": {
			Wsname:            "BTC/USD",
			Quote:             "ZUSD",
			FeeVolumeCurrency: "ZUSD",
			Fees: [][]float64{
				{0, 0.26},
				{50_000, 0.24},
			},
			FeesMaker: [][]float64{
				{0, 0.16},
			},
			TickSize: "0.1",
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	return catalog
}

func TestPairCatalogFeeRate(t *testing.T) {
	Convey("Given a catalog loaded from AssetPairs", t, func() {
		catalog := testPairCatalog(t)

		Convey("It should return published taker and maker rates", func() {
			taker, err := catalog.FeeRate("BTC/USD", trading.Market)

			So(err, ShouldBeNil)
			So(taker, ShouldAlmostEqual, 0.0026, 1e-12)

			maker, err := catalog.FeeRate("BTC/USD", trading.Limit)

			So(err, ShouldBeNil)
			So(maker, ShouldAlmostEqual, 0.0016, 1e-12)
		})

		Convey("It should advance fee tiers as volume accumulates", func() {
			So(catalog.RecordFill("BTC/USD", 50_000), ShouldBeNil)

			taker, err := catalog.FeeRate("BTC/USD", trading.Market)

			So(err, ShouldBeNil)
			So(taker, ShouldAlmostEqual, 0.0024, 1e-12)
		})

		Convey("It should reject unknown symbols", func() {
			_, err := catalog.FeeRate("ETH/USD", trading.Market)

			So(err, ShouldNotBeNil)
		})
	})
}
