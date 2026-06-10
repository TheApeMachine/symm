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
