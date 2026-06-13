package response

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
)

func TestTriggeredTakeProfitRestsUntilPrice(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a resting take-profit sell", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			if request.URL.Query().Get("pair") != "" {
				_, _ = writer.Write([]byte(`{
					"error": [],
					"result": {
						"XXBTZUSD": {
							"asks": [["51000.0", "1.0", 1781285552]],
							"bids": [["50000.0", "1.0", 1781285552]]
						}
					}
				}`))

				return
			}

			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZUSD": {
						"altname": "XBTUSD",
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
		catalog.depthAPI = public.EndpointType(server.URL)

		orders, err := NewOrders(ctx, pool, catalog)

		So(err, ShouldBeNil)

		balances := NewBalances(ctx, pool, catalog)

		_, fillErr := balances.ApplyFill(trading.AddParams{
			ClOrdID:   "entry-1",
			Symbol:    "BTC/USD",
			Side:      trading.Buy,
			OrderType: trading.Market,
			OrderQty:  0.001,
		}, 50_000)

		So(fillErr, ShouldBeNil)
		orders.Observe(balances)

		frame := types.KrakenMessage{
			Method: trading.MethodAddOrder,
			Params: &trading.AddParams{
				ClOrdID:   "tp-1",
				Symbol:    "BTC/USD",
				Side:      trading.Sell,
				OrderType: trading.TakeProfit,
				OrderQty:  0.001,
				Triggers: &trading.Triggers{
					Price:     0.02,
					PriceType: "pct",
				},
			},
		}

		orders.Send(&qpool.QValue[any]{Value: frame})

		Convey("It should not fill before the trigger price is reached", func() {
			So(len(orders.restingTriggered), ShouldEqual, 1)
			So(orders.DrainExecutions(), ShouldBeNil)

			orders.EvaluateTicker(&market.TickerUpdate{
				Symbol: "BTC/USD",
				Bid:    50_500,
			})

			So(orders.DrainExecutions(), ShouldBeNil)

			Convey("It should fill once the trigger price is crossed", func() {
				orders.EvaluateTicker(&market.TickerUpdate{
					Symbol: "BTC/USD",
					Bid:    51_500,
				})

				executions := orders.DrainExecutions()

				So(len(executions), ShouldEqual, 1)
				So(executions[0].Side, ShouldEqual, string(trading.Sell))
			})
		})
	})
}
