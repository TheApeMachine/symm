package response

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestBalancesApplyFillUsesAssetPairFees(t *testing.T) {
	Convey("Given a paper wallet backed by AssetPairs", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)

		catalog, err := NewPairCatalog(market.AssetPairs{
			"XXBTZUSD": {
				Wsname: "BTC/USD",
				Fees: [][]float64{
					{0, 0.26},
				},
				FeesMaker: [][]float64{
					{0, 0.16},
				},
			},
		})

		So(err, ShouldBeNil)

		balances, err := NewBalances(ctx, pool, catalog)

		So(err, ShouldBeNil)

		execution, fillErr := balances.ApplyFill(trading.AddParams{
			ClOrdID:   "order-1",
			Symbol:    "BTC/USD",
			Side:      trading.Buy,
			OrderType: trading.Market,
			OrderQty:  0.001,
		}, 50_000)

		Convey("It should charge the published taker fee", func() {
			So(fillErr, ShouldBeNil)
			So(execution.FeeUsdEquiv, ShouldAlmostEqual, 0.13, 1e-9)
			So(execution.LiquidityInd, ShouldEqual, "t")
			So(balances.Wallet().Asset[0].Balance, ShouldAlmostEqual, 200-50-0.13, 1e-9)
		})
	})
}

func BenchmarkPairCatalogFeeRate(b *testing.B) {
	catalog, err := NewPairCatalog(market.AssetPairs{
		"XXBTZUSD": {
			Wsname: "BTC/USD",
			Fees: [][]float64{
				{0, 0.26},
			},
			FeesMaker: [][]float64{
				{0, 0.16},
			},
		},
	})

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = catalog.FeeRate("BTC/USD", trading.Market)
	}
}
