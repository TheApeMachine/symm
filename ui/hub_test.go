package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fastws "github.com/fasthttp/websocket"
	fiberws "github.com/gofiber/contrib/v3/websocket"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
TestHubWriteFrontend proves publication is safe before any dashboard client
connects. The guard must return without touching the nil connection — it
previously checked `hub.frontend != nil` and fell through to WriteMessage on
the nil connection, panicking on the first observe tick.

The hub no longer runs on the ring, so this is no longer about protecting the
pipeline from the encode; it is about the publisher goroutine surviving a run
with nobody watching. The encode itself must not happen at all in that case,
which the allocation count proves.
*/
func TestHubWriteFrontend(t *testing.T) {
	Convey("Given a hub with no dashboard clients", t, func() {
		hub := &Hub{}
		measurement := data.NewMeasurement[float64]("cvd", nil)
		measurement.Label = "TEST/USD"

		Convey("Writing returns without panicking", func() {
			So(func() { hub.writeFrontend(measurement) }, ShouldNotPanic)
		})

		Convey("Writing does not allocate a discarded FlatBuffer snapshot", func() {
			allocations := testing.AllocsPerRun(100, func() {
				hub.writeFrontend(measurement)
			})

			So(allocations, ShouldEqual, 0)
		})
	})
	Convey("A failed browser write detaches the dead connection immediately", t, func() {
		accepted := make(chan *fastws.Conn, 1)
		upgrader := fastws.Upgrader{}
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			connection, err := upgrader.Upgrade(response, request, nil)
			if err != nil {
				t.Error(err)
				return
			}
			accepted <- connection
		}))
		defer server.Close()
		client, response, err := fastws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		So(err, ShouldBeNil)
		if response.Body != nil {
			defer response.Body.Close()
		}
		defer client.Close()
		connection := <-accepted
		So(connection.Close(), ShouldBeNil)
		hub := &Hub{frontend: &fiberws.Conn{Conn: connection}}
		measurement := data.NewMeasurement[float64]("cvd", nil)
		measurement.Label = "TEST/USD"
		hub.writeFrontend(measurement)
		So(hub.frontend, ShouldBeNil)
		So(testing.AllocsPerRun(10, func() { hub.writeFrontend(measurement) }), ShouldEqual, 0)
	})
}

func TestHubSetHindsightStore(t *testing.T) {
	Convey("The Hindsight canonical HTTP contract survives an Iceberg round trip", t, func() {
		catalog := tablestest.New(t)
		writer := tables.NewWriter(catalog)
		at := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
		price, err := decimal.NewFromString("50000.5")
		So(err, ShouldBeNil)
		qty, err := decimal.NewFromString("1.5")
		So(err, ShouldBeNil)
		fee, err := decimal.NewFromString("0.15")
		So(err, ShouldBeNil)

		writer.AddSpotTicker(tables.SpotTickerRow{
			Epoch: 1, Tick: 10, Symbol: "BTC/USD", VenueAt: at, ReceivedAt: at,
			Bid: 50000, Ask: 50001, Last: 50000.5,
		})
		writer.AddSpotTrade(tables.SpotTradeRow{
			Epoch: 1, Tick: 11, Symbol: "BTC/USD", VenueAt: at, ReceivedAt: at,
			Price: 50000.5, Qty: 1.5, Side: "buy", OrdType: "limit", TradeID: 12345,
		})
		writer.AddSpotLevel3(tables.SpotLevel3Row{
			Epoch: 1, Tick: 12, Symbol: "BTC/USD", VenueAt: at, ReceivedAt: at,
			Side: "buy", Event: "add", OrderID: "O1", LimitPrice: 50000, OrderQty: 2.0,
		})
		writer.AddExecution(tables.ExecutionRow{
			Epoch: 1, Tick: 13, Symbol: "BTC/USD", OrderID: "ord-1", Side: "buy",
			OrderStatus: "filled", LastPrice: price, LastQty: qty, Cost: price, FeeUsdEquiv: fee,
		})
		writer.AddMeasurement(tables.MeasurementRow{
			Epoch: 1, Tick: 14, Source: "cvd", Symbol: "BTC/USD", VenueAt: at,
			ObservedAt: at, Maturity: 1.0, SNR: 2.5, SNRDefined: true,
			Metrics: map[string]float64{"delta": 100.0},
		})

		So(writer.Commit(t.Context()), ShouldBeNil)

		hub := NewHub(t.Context())
		hub.SetHindsightStore(catalog)
		t.Cleanup(func() {
			if err := hub.Close(); err != nil {
				t.Error(err)
			}
		})

		read := func(path string, target any) {
			response, err := hub.app.Test(httptest.NewRequest("GET", path, nil))
			So(err, ShouldBeNil)
			So(response.StatusCode, ShouldEqual, 200)
			So(json.NewDecoder(response.Body).Decode(target), ShouldBeNil)
			So(response.Body.Close(), ShouldBeNil)
		}

		Convey("Spot ticker records are queryable by epoch and tick", func() {
			var tickers []tables.SpotTickerRow
			read("/hindsight/spot_ticker?epoch=1&after=0", &tickers)
			So(len(tickers), ShouldEqual, 1)
			So(tickers[0].Symbol, ShouldEqual, "BTC/USD")
			So(tickers[0].Last, ShouldEqual, 50000.5)
		})

		Convey("Spot trade records are queryable by epoch and tick", func() {
			var trades []tables.SpotTradeRow
			read("/hindsight/spot_trade?epoch=1&after=0", &trades)
			So(len(trades), ShouldEqual, 1)
			So(trades[0].Symbol, ShouldEqual, "BTC/USD")
			So(trades[0].TradeID, ShouldEqual, 12345)
		})

		Convey("Spot Level3 records are queryable by epoch and tick", func() {
			var level3Rows []tables.SpotLevel3Row
			read("/hindsight/spot_level3?epoch=1&after=0", &level3Rows)
			So(len(level3Rows), ShouldEqual, 1)
			So(level3Rows[0].OrderID, ShouldEqual, "O1")
			So(level3Rows[0].LimitPrice, ShouldEqual, 50000)
		})

		Convey("Execution records are queryable by epoch and tick", func() {
			var execs []tables.ExecutionRow
			read("/hindsight/executions?epoch=1&after=0", &execs)
			So(len(execs), ShouldEqual, 1)
			So(execs[0].OrderID, ShouldEqual, "ord-1")
			So(execs[0].Side, ShouldEqual, "buy")
		})

		Convey("Measurement records are queryable by epoch and tick", func() {
			var measurements []tables.MeasurementRow
			read("/hindsight/measurements?epoch=1&after=0", &measurements)
			So(len(measurements), ShouldEqual, 1)
			So(measurements[0].Source, ShouldEqual, "cvd")
			So(measurements[0].Metrics["delta"], ShouldEqual, 100.0)
		})
	})
}
