package websocket

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

/*
TestLifecycleReplay verifies that paper history emits the exchange fill once,
without inventing partial executions that are absent from the source record.
*/
func TestLifecycleReplay(t *testing.T) {
	Convey("Given one paper trade from exchange history", t, func() {
		paper := NewPaper(context.Background(), NewSimulator())
		var replayed *kraken.Execution
		paper.On("executions", func(buffer []byte) {
			replayed = kraken.NewExecution(buffer)
		})

		err := paper.lifecycle.Replay([]any{map[string]any{
			"id": "PAPER-00026", "order_id": "PAPER-00025",
			"pair": "BTCUSD", "side": "buy", "volume": 0.0002,
			"price": 64129.9, "cost": 12.82598, "fee": 0.033347548,
			"status": "filled", "time": "2026-07-14T21:02:56Z",
		}})

		Convey("Then the original execution identity and fee are preserved", func() {
			So(err, ShouldBeNil)
			So(replayed, ShouldNotBeNil)
			So(replayed.Data, ShouldHaveLength, 1)
			So(replayed.Data[0].ExecID, ShouldEqual, "PAPER-00026")
			So(replayed.Data[0].OrderStatus, ShouldEqual, "filled")
			So(replayed.Data[0].FeeUsdEquiv.Float64(), ShouldAlmostEqual, 0.033347548, 1e-8)
		})
	})
}

/*
BenchmarkLifecycleReplay measures conversion and emission of one real paper fill.
*/
func BenchmarkLifecycleReplay(b *testing.B) {
	paper := NewPaper(context.Background(), NewSimulator())
	paper.On("executions", func([]byte) {})
	trades := []any{map[string]any{
		"id": "PAPER-00026", "order_id": "PAPER-00025",
		"pair": "BTCUSD", "side": "buy", "volume": 0.0002,
		"price": 64129.9, "cost": 12.82598, "fee": 0.033347548,
		"status": "filled", "time": "2026-07-14T21:02:56Z",
	}}

	for b.Loop() {
		if err := paper.lifecycle.Replay(trades); err != nil {
			b.Fatal(err)
		}
	}
}
