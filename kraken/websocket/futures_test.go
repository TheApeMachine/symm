package websocket

import (
	"context"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func newTestFutures(ctx context.Context) *FuturesLive {
	futures := &FuturesLive{
		System:        runtime.NewSystem(ctx, "websocket:futures"),
		callbacks:     &sync.Map{},
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
		})

		Convey("Register declares the schema", func() {
			registered := futures.Register()
			So(registered, ShouldNotBeNil)
			So(registered.Source, ShouldEqual, "futures")
		})
	})
}

func TestFuturesLiveStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &FuturesLive{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(sequence.Read[*data.Measurement[float64]](node.Next(sequence.NewValue(measurement))), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}
