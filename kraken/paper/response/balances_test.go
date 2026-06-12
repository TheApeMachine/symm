package response

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestBalancesApplyFillUsesAssetPairFees(t *testing.T) {
	Convey("Given a paper wallet backed by AssetPairs REST", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZUSD": {
						"wsname": "BTC/USD",
						"fees": [[0, 0.26]],
						"fees_maker": [[0, 0.16]]
					}
				}
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)

		catalog := NewPairCatalog(ctx)
		catalog.assetPairsAPI = public.EndpointType(server.URL)

		balances := NewBalances(ctx, pool, catalog)
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

func TestBalancesApplyFillTracksHoldings(t *testing.T) {
	Convey("Given a funded paper wallet", t, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZUSD": {
						"wsname": "BTC/USD",
						"fees": [[0, 0.26]],
						"fees_maker": [[0, 0.16]]
					}
				}
			}`))
		}))
		defer server.Close()

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)

		catalog := NewPairCatalog(ctx)
		catalog.assetPairsAPI = public.EndpointType(server.URL)

		balances := NewBalances(ctx, pool, catalog)

		Convey("Selling without holdings is rejected", func() {
			_, sellErr := balances.ApplyFill(trading.AddParams{
				ClOrdID:   "sell-flat",
				Symbol:    "BTC/USD",
				Side:      trading.Sell,
				OrderType: trading.Market,
				OrderQty:  0.001,
			}, 50_000)

			So(sellErr, ShouldEqual, ErrInsufficientHoldings)
		})

		Convey("A buy credits the base asset and a sell realizes P&L", func() {
			_, buyErr := balances.ApplyFill(trading.AddParams{
				ClOrdID:   "buy-1",
				Symbol:    "BTC/USD",
				Side:      trading.Buy,
				OrderType: trading.Market,
				OrderQty:  0.001,
			}, 50_000)

			So(buyErr, ShouldBeNil)

			wallet := balances.Wallet()
			So(len(wallet.Asset), ShouldEqual, 2)
			So(wallet.Asset[1].Asset, ShouldEqual, "BTC")
			So(wallet.Asset[1].Balance, ShouldAlmostEqual, 0.001, 1e-12)

			_, oversellErr := balances.ApplyFill(trading.AddParams{
				ClOrdID:   "sell-too-much",
				Symbol:    "BTC/USD",
				Side:      trading.Sell,
				OrderType: trading.Market,
				OrderQty:  0.002,
			}, 52_000)

			So(oversellErr, ShouldEqual, ErrInsufficientHoldings)

			_, sellErr := balances.ApplyFill(trading.AddParams{
				ClOrdID:   "sell-1",
				Symbol:    "BTC/USD",
				Side:      trading.Sell,
				OrderType: trading.Market,
				OrderQty:  0.001,
			}, 52_000)

			So(sellErr, ShouldBeNil)

			// Buy: 200 - 50 - 0.13 fee; sell: +52 - 0.1352 fee.
			wallet = balances.Wallet()
			So(wallet.Asset[0].Balance, ShouldAlmostEqual, 200-50-0.13+52-0.1352, 1e-9)
			So(wallet.Asset[1].Balance, ShouldAlmostEqual, 0, 1e-12)

			realized, _ := balances.realized.Float64()
			So(realized, ShouldAlmostEqual, 52-0.1352-(50+0.13), 1e-9)
		})
	})
}
