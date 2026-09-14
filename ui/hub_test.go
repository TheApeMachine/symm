package ui

import (
	"context"
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
	"github.com/theapemachine/symm/tests/tablestest"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
	"golang.design/x/lockfree/wf"
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
		types.SetFocus("TEST/USD")
		defer types.SetFocus("BTC/USD")
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

		Convey("Hindsight runs are queryable", func() {
			var runs []tables.HindsightRun
			read("/hindsight/runs", &runs)
			So(len(runs), ShouldEqual, 1)
			So(runs[0].ID, ShouldEqual, "1")
		})

		Convey("Hindsight timeline is queryable", func() {
			var timeline tables.HindsightTimeline
			read("/hindsight/timeline?run=1&symbol=BTC/USD&buckets=10", &timeline)
			So(timeline.Symbol, ShouldEqual, "BTC/USD")
			So(len(timeline.Buckets), ShouldEqual, 10)
		})

		Convey("Hindsight lifecycle is queryable", func() {
			var lifecycle []tables.HindsightLifecycleEvent
			read("/hindsight/lifecycle?run=1", &lifecycle)
			So(len(lifecycle), ShouldBeGreaterThanOrEqualTo, 1)
		})

		Convey("Hindsight captures are queryable", func() {
			var captures []tables.HindsightCapture
			read("/hindsight/captures?run=1&after=0", &captures)
			So(len(captures), ShouldBeGreaterThanOrEqualTo, 1)
		})

		Convey("Hindsight envelope is queryable", func() {
			var envelope tables.HindsightEnvelope
			read("/hindsight/envelope?run=1&seq=10", &envelope)
			So(envelope.Sequence, ShouldEqual, 10)
		})

		Convey("Hindsight resident is queryable", func() {
			var resident tables.HindsightResident
			read("/hindsight/resident?run=1&symbol=BTC/USD&seq=15&budget=10", &resident)
			So(resident.Sequence, ShouldEqual, 15)
		})

		Convey("Hindsight metric map is queryable", func() {
			var metricMap map[string]any
			read("/hindsight/metric-map", &metricMap)
			So(metricMap["metrics"], ShouldNotBeNil)
		})
	})
}

func TestHubDrain(t *testing.T) {
	Convey("Given a hub draining from a wait-free ring buffer", t, func() {
		hub := NewHub(t.Context())
		t.Cleanup(func() {
			if err := hub.Close(); err != nil {
				t.Error(err)
			}
		})

		Convey("A nil ring buffer returns immediately without panicking", func() {
			So(func() { hub.Drain(nil) }, ShouldNotPanic)
		})

		Convey("Measurements placed on the ring are drained without leaking", func() {
			ring := wf.NewRingBuffer[*data.Measurement[float64]](64)

			for index := 0; index < 10; index++ {
				measurement := data.NewMeasurement[float64]("cvd", nil)
				measurement.Label = "BTC/USD"
				measurement.SeqIdx = int64(index + 1)
				ring.Put(measurement)
			}

			So(ring.IsEmpty(), ShouldBeFalse)

			done := make(chan struct{})
			go func() {
				hub.Drain(ring)
				close(done)
			}()

			time.Sleep(50 * time.Millisecond)
			So(ring.IsEmpty(), ShouldBeTrue)

		hub.cancel()
			<-done
		})
	})
}

func TestIsRawMarketData(t *testing.T) {
	Convey("Given measurements from various sources and channels", t, func() {
		Convey("Nil measurement is treated as raw/invalid", func() {
			So(IsRawMarketData(nil), ShouldBeTrue)
			So(IsAllowedTelemetry(nil), ShouldBeFalse)
		})

		Convey("Spot websocket source measurements are recognized as raw", func() {
			spotMeas := data.NewMeasurement[float64]("websocket", nil)
			So(IsRawMarketData(spotMeas), ShouldBeTrue)

			publicMeas := data.NewMeasurement[float64]("public", nil)
			So(IsRawMarketData(publicMeas), ShouldBeTrue)
		})

		Convey("Spot ticker, trade, and level3 channels are recognized as raw", func() {
			tickerMeas := data.NewMeasurement[float64]("feed", nil)
			tickerMeas.Provenance = map[string]string{"channel": "ticker"}
			So(IsRawMarketData(tickerMeas), ShouldBeTrue)

			tradeMeas := data.NewMeasurement[float64]("feed", nil)
			tradeMeas.Provenance = map[string]string{"channel": "trade"}
			So(IsRawMarketData(tradeMeas), ShouldBeTrue)

			level3Meas := data.NewMeasurement[float64]("feed", nil)
			level3Meas.Provenance = map[string]string{"channel": "level3"}
			So(IsRawMarketData(level3Meas), ShouldBeTrue)

			bookMeas := data.NewMeasurement[float64]("feed", nil)
			bookMeas.Provenance = map[string]string{"channel": "book"}
			So(IsRawMarketData(bookMeas), ShouldBeTrue)
		})

		Convey("Futures source and channels are recognized as raw", func() {
			futuresMeas := data.NewMeasurement[float64]("futures", nil)
			So(IsRawMarketData(futuresMeas), ShouldBeTrue)

			futuresChannelMeas := data.NewMeasurement[float64]("feed", nil)
			futuresChannelMeas.Provenance = map[string]string{"channel": "futures.ticker"}
			So(IsRawMarketData(futuresChannelMeas), ShouldBeTrue)
		})

		Convey("Analytical signal and category measurements are admitted", func() {
			categoryMeas := data.NewMeasurement[float64]("category", nil)
			categoryMeas.Label = "BTC/USD"
			So(IsRawMarketData(categoryMeas), ShouldBeFalse)
			So(IsAllowedTelemetry(categoryMeas), ShouldBeTrue)

			cvdMeas := data.NewMeasurement[float64]("cvd", nil)
			cvdMeas.Label = "BTC/USD"
			So(IsRawMarketData(cvdMeas), ShouldBeFalse)
			So(IsAllowedTelemetry(cvdMeas), ShouldBeTrue)

			hawkesMeas := data.NewMeasurement[float64]("hawkes", nil)
			hawkesMeas.Label = "BTC/USD"
			So(IsRawMarketData(hawkesMeas), ShouldBeFalse)
			So(IsAllowedTelemetry(hawkesMeas), ShouldBeTrue)
		})
	})
}

func TestIsWireAllowed(t *testing.T) {
	Convey("Given a hub and focus-gated measurements", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		hub := NewHub(ctx)
		defer hub.Close()

		original := types.Focus()
		Reset(func() {
			types.SetFocus(original)
		})

		Convey("Raw market data is always rejected regardless of focus", func() {
			types.SetFocus("BTC/USD")
			rawMeas := data.NewMeasurement[float64]("websocket", nil)
			rawMeas.Label = "BTC/USD"
			So(hub.isWireAllowed(rawMeas), ShouldBeFalse)
			So(hub.isWireAllowed(nil), ShouldBeFalse)
		})

		Convey("Training and system measurements are admitted regardless of focus", func() {
			types.SetFocus("BTC/USD")

			trainingMeas := data.NewMeasurement[float64]("training", nil)
			trainingMeas.Label = "ETH/USD"
			So(hub.isWireAllowed(trainingMeas), ShouldBeTrue)

			globalMeas := data.NewMeasurement[float64]("pulse", nil)
			globalMeas.Label = ""
			So(hub.isWireAllowed(globalMeas), ShouldBeTrue)
		})

		Convey("Signal measurements are filtered by the active focus symbol", func() {
			types.SetFocus("BTC/USD")

			btcMeas := data.NewMeasurement[float64]("cvd", nil)
			btcMeas.Label = "BTC/USD"
			So(hub.isWireAllowed(btcMeas), ShouldBeTrue)

			ethMeas := data.NewMeasurement[float64]("cvd", nil)
			ethMeas.Label = "ETH/USD"
			So(hub.isWireAllowed(ethMeas), ShouldBeFalse)

			solMeas := data.NewMeasurement[float64]("hawkes", nil)
			solMeas.Label = "SOL/USD"
			So(hub.isWireAllowed(solMeas), ShouldBeFalse)
		})

		Convey("Changing the focus dynamically admits the new symbol", func() {
			types.SetFocus("ETH/USD")

			btcMeas := data.NewMeasurement[float64]("cvd", nil)
			btcMeas.Label = "BTC/USD"
			So(hub.isWireAllowed(btcMeas), ShouldBeFalse)

			ethMeas := data.NewMeasurement[float64]("cvd", nil)
			ethMeas.Label = "ETH/USD"
			So(hub.isWireAllowed(ethMeas), ShouldBeTrue)
		})

		Convey("Wildcard or empty focus admits all analytical signals", func() {
			types.SetFocus("*")

			btcMeas := data.NewMeasurement[float64]("cvd", nil)
			btcMeas.Label = "BTC/USD"
			So(hub.isWireAllowed(btcMeas), ShouldBeTrue)

			ethMeas := data.NewMeasurement[float64]("cvd", nil)
			ethMeas.Label = "ETH/USD"
			So(hub.isWireAllowed(ethMeas), ShouldBeTrue)
		})
	})
}

