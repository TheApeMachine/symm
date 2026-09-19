package websocket

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

func makeLiveEvent(raw []byte) *callback.Event[*sdk.WebSocketMessage] {
	return &callback.Event[*sdk.WebSocketMessage]{
		Data: sdk.NewWebSocketMessage(raw),
	}
}

func TestLiveStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		called := false
		node := &Live{
			System: runtime.NewSystem(t.Context(), "readiness-test"),
			pipeline: func(in any) any {
				called = true
				return in
			},
		}

		raw := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"ETH/USD","last":101.5}]}`)

		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			node.onReceived(makeLiveEvent(raw))
			So(called, ShouldBeFalse)
			So(node.Status(), ShouldEqual, stage)
		}
	})
}

func TestLiveConnections(t *testing.T) {
	Convey("Root activates Level 3 owners explicitly after preparing consumers", t, func() {
		parent := &Live{System: runtime.NewSystem(t.Context(), "private")}
		child := &Live{System: runtime.NewSystem(t.Context(), "level3")}
		child.Transition(runtime.BUSY)
		parent.AttachLevel3("BTC/USD", child)
		So(parent.Connections(), ShouldResemble, []*Live{child})
		So(child.Status(), ShouldEqual, runtime.BUSY)
		parent.Transition(runtime.READY)
		So(child.Status(), ShouldEqual, runtime.BUSY)

		for _, connection := range parent.Connections() {
			connection.Transition(runtime.READY)
		}

		So(child.Status(), ShouldEqual, runtime.READY)
		parent.Transition(runtime.WAITING)
		So(child.Status(), ShouldEqual, runtime.READY)
	})
}

func TestLiveWritesTickerToGrid(t *testing.T) {
	Convey("A public ticker row is written to the grid", t, func() {
		keyA := store.NewKey[any]("ticker", "data", "last")
		cellA := func(in any) float64 {
			if val := keyA(in); val != nil {
				return *val
			}
			return 0
		}

		grid := store.NewGrid[any, float64]()
		var zero any
		grid(transport.NewMessage[any, float64](transport.REGISTER, zero, types.Value[any, float64](cellA)))

		live := &Live{
			System: runtime.NewSystem(t.Context(), "public"),
			pipeline: func(in any) any {
				grid(transport.NewMessage[any, float64](transport.POKE, in, nil))
				return nil
			},
		}
		live.Transition(runtime.READY)

		raw := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"ETH/USD","last":101.5}]}`)
		live.onReceived(makeLiveEvent(raw))

		readings := grid(transport.NewMessage[any, float64](transport.PEEK, zero, nil))
		So(readings, ShouldNotBeNil)
		So(len(readings), ShouldEqual, 1)
		So(readings[0], ShouldEqual, 101.5)
	})
}
