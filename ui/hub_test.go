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
		at := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)

		err := catalog.RecordRun(t.Context(), tables.Run{
			Epoch:     1,
			StartedAt: at,
			BuildID:   "test",
			Status:    "ACTIVE",
		})
		So(err, ShouldBeNil)

		writer := tables.NewWriter(catalog, 1)

		tickerMeasurement := &data.Measurement[float64]{
			Source:   "spot_ticker",
			Label:    "BTC/USD",
			SeqIdx:   10,
			At:       at,
			Maturity: 1.0,
			Metrics: map[string]data.Metric[float64]{
				"bid":  {Label: "bid", Raw: 50000},
				"ask":  {Label: "ask", Raw: 50001},
				"last": {Label: "last", Raw: 50000.5},
			},
		}
		writer.Add("ticker", tickerMeasurement)

		tradeMeasurement := &data.Measurement[float64]{
			Source:   "spot_trade",
			Label:    "BTC/USD",
			SeqIdx:   11,
			At:       at,
			Maturity: 1.0,
			Metrics: map[string]data.Metric[float64]{
				"price": {Label: "price", Raw: 50000.5},
				"qty":   {Label: "qty", Raw: 1.5},
			},
		}
		writer.Add("trade", tradeMeasurement)

		So(writer.CommitReady(t.Context(), true), ShouldBeNil)

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

		Convey("Hindsight runs are queryable", func() {
			var runs []tables.Run
			read("/hindsight/runs", &runs)
			So(len(runs), ShouldEqual, 1)
			So(runs[0].Epoch, ShouldEqual, 1)
		})

		Convey("Hindsight timeline is queryable", func() {
			var timeline []*data.Measurement[float64]
			read("/hindsight/timeline?run=1&symbol=BTC/USD", &timeline)
			So(len(timeline), ShouldEqual, 1)
			So(timeline[0].Label, ShouldEqual, "BTC/USD")
		})

		Convey("Hindsight symbols are queryable", func() {
			var symbols []string
			read("/hindsight/symbols?run=1", &symbols)
			So(len(symbols), ShouldEqual, 1)
			So(symbols[0], ShouldEqual, "BTC/USD")
		})

		Convey("Hindsight data is queryable by table", func() {
			var dataRows []*data.Measurement[float64]
			read("/hindsight/data?epoch=1&table=spot_ticker", &dataRows)
			So(len(dataRows), ShouldEqual, 1)
			So(dataRows[0].Label, ShouldEqual, "BTC/USD")
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

