package response

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
)

func TestOrdersSend(t *testing.T) {
	testconfig.Load(t)

	Convey("Given an add_order frame", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		orders, err := NewOrders(ctx, pool, NewPairCatalog(ctx))

		So(err, ShouldBeNil)
		So(orders, ShouldNotBeNil)

		frame := types.KrakenMessage{
			Method: trading.MethodAddOrder,
			Params: &trading.AddParams{ClOrdID: "d7ce4944-f4df-4447-8314-14f020"},
			ReqID:  0,
		}

		response := orders.Send(&qpool.QValue[any]{Value: frame})

		Convey("It should marshal order updates as an array", func() {
			So(response, ShouldNotBeNil)
			So(response.Channel, ShouldEqual, "orders")

			decoded := []trading.OrderUpdate{}

			So(response.Unmarshal(&decoded), ShouldBeNil)
			So(len(decoded), ShouldEqual, 1)
			So(decoded[0].OrderID, ShouldEqual, "d7ce4944-f4df-4447-8314-14f020")
		})
	})
}

func TestOrdersSendDrainsMarketExecution(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a paper market order backed by mixed-type depth rows", t, func() {
		server := httptest.NewServer(http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			if request.URL.Query().Get("pair") != "" {
				_, _ = writer.Write([]byte(`{
					"error": [],
					"result": {
						"XXBTZUSD": {
							"asks": [["50000.0", "1.0", 1781285552]],
							"bids": [["49900.0", "1.0", 1781285552]]
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
		So(orders, ShouldNotBeNil)

		balances := NewBalances(ctx, pool, catalog)
		orders.Observe(balances)

		frame := types.KrakenMessage{
			Method: trading.MethodAddOrder,
			Params: &trading.AddParams{
				ClOrdID:    "order-1",
				Symbol:     "BTC/USD",
				Side:       trading.Buy,
				OrderType:  trading.Market,
				OrderQty:   0.001,
				LimitPrice: 49000,
			},
			ReqID: 0,
		}

		response := orders.Send(&qpool.QValue[any]{Value: frame})
		executions := orders.DrainExecutions()
		wallet := balances.Wallet()

		Convey("It should fill and price the position from the same depth", func() {
			So(response, ShouldNotBeNil)
			So(len(executions), ShouldEqual, 1)
			So(executions[0].AvgPrice, ShouldEqual, 50000)
			So(orders.DrainExecutions(), ShouldBeNil)
			So(wallet.Marks["BTC/USD"], ShouldAlmostEqual, 49900, 1e-9)
			So(wallet.ExitFeeRate["BTC"], ShouldAlmostEqual, 0.0026, 1e-12)
			So(wallet.Expected["BTC"], ShouldAlmostEqual, 49.9*0.9974, 1e-9)
			So(wallet.Unrealized["BTC"], ShouldAlmostEqual, (49.9*0.9974)-50.13, 1e-9)
		})
	})
}

func BenchmarkOrdersMarketFillQuote(benchmark *testing.B) {
	testconfig.MustLoad()

	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Query().Get("pair") != "" {
			_, _ = writer.Write([]byte(`{
				"error": [],
				"result": {
					"XXBTZUSD": {
						"asks": [["50000.0", "1.0", 1781285552]],
						"bids": [["49900.0", "1.0", 1781285552]]
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
	benchmark.Cleanup(server.Close)

	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 4, nil)
	catalog := NewPairCatalog(ctx)
	catalog.assetPairsAPI = public.EndpointType(server.URL)
	catalog.depthAPI = public.EndpointType(server.URL)

	orders, err := NewOrders(ctx, pool, catalog)

	if err != nil {
		benchmark.Fatal(err)
	}

	params := trading.AddParams{
		ClOrdID:   "order-1",
		Symbol:    "BTC/USD",
		Side:      trading.Buy,
		OrderType: trading.Market,
		OrderQty:  0.001,
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, quoteErr := orders.marketFillQuote(params, params.OrderQty)

		if quoteErr != nil {
			benchmark.Fatal(quoteErr)
		}
	}
}
