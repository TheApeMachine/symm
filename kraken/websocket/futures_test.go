package websocket

import (
	"context"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.design/x/lockfree/lf"
)

func newTestFutures(ctx context.Context) *FuturesLive {
	futures := &FuturesLive{
		System:        runtime.NewSystem(ctx, "websocket:futures"),
		callbacks:     &sync.Map{},
		queue:         lf.NewQueue[map[string]any](),
		subscriptions: make(map[string][]string),
	}

	futures.Transition(runtime.READY)
	return futures
}

func makeFuturesEvent(raw []byte) *callback.Event[*sdkkraken.WebSocketMessage] {
	return &callback.Event[*sdkkraken.WebSocketMessage]{
		Data: sdkkraken.NewWebSocketMessage(raw),
	}
}

func TestFuturesLive(t *testing.T) {
	Convey("Given a FuturesLive transport session", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		futures := newTestFutures(ctx)

		futures.SetResolver(func(productID string) (string, bool) {
			if productID == "PI_XBTUSD" {
				return "BTC/USD", true
			}

			return "", false
		})

		Convey("When an alert frame arrives", func() {
			event := makeFuturesEvent([]byte(`{"event":"alert","message":"Already subscribed to feed, re-requesting"}`))
			futures.onReceived(event)

			Convey("The session must remain alive without transitioning to error", func() {
				So(futures.Status(), ShouldEqual, runtime.READY)
				So(futures.Error(), ShouldBeNil)
			})
		})

		Convey("When an error frame arrives", func() {
			event := makeFuturesEvent([]byte(`{"event":"error","message":"malformed product"}`))
			futures.onReceived(event)

			Convey("The session should transition to error", func() {
				So(futures.Status(), ShouldEqual, runtime.ERROR)
				So(futures.Error(), ShouldNotBeNil)
			})
		})

		Convey("When Transition to READY is called", func() {
			futures.Transition(runtime.READY)

			Convey("The session transitions to ready", func() {
				So(futures.Status(), ShouldEqual, runtime.READY)
			})

			Convey("And a ticker frame is received and stepped", func() {
				event := makeFuturesEvent([]byte(`{"feed":"ticker","product_id":"PI_XBTUSD","bid":50000.0,"ask":50001.0,"last":50000.5,"markPrice":50000.2,"index":50000.1,"openInterest":1000.0}`))
				futures.onReceived(event)

				inputMeasurement := data.NewMeasurement("futures", map[string]data.Metric[float64]{
					"last":        data.NewMetric[float64]("last", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
					"last_price":  data.NewMetric[float64]("last_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
					"index_price": data.NewMetric[float64]("index_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
					"mark_price":  data.NewMetric[float64]("mark_price", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
				})

				stepped := futures.Step(inputMeasurement)
				So(stepped.Label, ShouldEqual, "BTC/USD")
				So(stepped.Metrics["last"].Raw, ShouldEqual, 50000.5)
				So(stepped.Metrics["last_price"].Raw, ShouldEqual, 50000.5)
				So(stepped.Metrics["index_price"].Raw, ShouldEqual, 50000.1)
				So(stepped.Metrics["mark_price"].Raw, ShouldEqual, 50000.2)
			})

			Convey("And a trade frame is received and stepped", func() {
				event := makeFuturesEvent([]byte(`{"feed":"trade","product_id":"PI_XBTUSD","side":"buy","type":"fill","price":50000.5,"qty":2.5,"uid":"trade-123"}`))
				futures.onReceived(event)

				inputMeasurement := data.NewMeasurement("futures", map[string]data.Metric[float64]{
					"price": data.NewMetric[float64]("price", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
					"qty":   data.NewMetric[float64]("qty", data.UnitRate, data.TimescaleInstantaneous, 0, 1),
				})

				stepped := futures.Step(inputMeasurement)
				So(stepped.Label, ShouldEqual, "BTC/USD")
				So(stepped.Metrics["price"].Raw, ShouldEqual, 50000.5)
				So(stepped.Metrics["qty"].Raw, ShouldEqual, 2.5)
				So(stepped.Provenance["side"], ShouldEqual, "buy")
				So(stepped.Provenance["type"], ShouldEqual, "fill")
			})
		})

		Convey("Register declares the schema", func() {
			registered := futures.Register()
			So(registered, ShouldNotBeNil)
			So(registered.Source, ShouldEqual, "futures")
		})
	})
}
