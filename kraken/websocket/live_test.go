package websocket

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func makeLiveEvent(raw []byte) *callback.Event[*sdk.WebSocketMessage] {
	return &callback.Event[*sdk.WebSocketMessage]{
		Data: sdk.NewWebSocketMessage(raw),
	}
}

func TestLiveStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Live{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(sequence.Read[*data.Measurement[float64]](node.Next(sequence.NewValue(measurement))), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
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
	Convey("A public ticker row is written to the grid through Query Write", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()
		held := store.NewRetained[float64]()
		conn := transport.NewConn[*geometry.Coordinate](
			nomagique.NewNumber(store.NewField("ticker", "data", "last"), held),
		)
		sequence.Read[core.Connectable[*geometry.Coordinate]](grid.Next(
			core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				conn, core.Identify,
			).Next(sequence.NewValue([][]string{{"ticker", "data", "last"}})),
		))

		live := &Live{
			System: runtime.NewSystem(t.Context(), "public"),
			grid:   grid,
		}
		live.Transition(runtime.READY)

		raw := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"ETH/USD","last":101.5}]}`)
		live.onReceived(makeLiveEvent(raw))

		reading := sequence.Read[core.Input[*geometry.Coordinate, string, float64]](grid.Next(
			core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				conn, core.Read,
			).Next(nil),
		))
		So(reading.Value, ShouldNotBeNil)
		So(*reading.Value, ShouldEqual, 101.5)
	})
}
