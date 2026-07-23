package websocket

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
)

/*
TestPaperReplay verifies that paper history emits the exchange fill once,
without inventing partial executions that are absent from the source record.
*/
func TestPaperReplay(t *testing.T) {
	Convey("Given one paper trade from exchange history", t, func() {
		viper.Set("system.actor.buffer", 64)
		paper := NewPaper(context.Background(), NewSimulator())
		sub := paper.Subscribe("executions")

		err := paper.Replay([]any{map[string]any{
			"id": "PAPER-00026", "order_id": "PAPER-00025",
			"pair": "BTCUSD", "side": "buy", "volume": 0.0002,
			"price": 64129.9, "cost": 12.82598, "fee": 0.033347548,
			"status": "filled", "time": "2026-07-14T21:02:56Z",
		}})

		Convey("Then the original execution identity and fee are preserved", func() {
			So(err, ShouldBeNil)
			replayed := kraken.NewExecution((<-sub.Channel).([]byte))
			So(replayed.Data, ShouldHaveLength, 1)
			So(replayed.Data[0].ExecID, ShouldEqual, "PAPER-00026")
			So(replayed.Data[0].OrderStatus, ShouldEqual, "filled")
			So(replayed.Data[0].FeeUsdEquiv.Float64(), ShouldAlmostEqual, 0.033347548, 1e-8)
		})
	})
}

/*
BenchmarkPaperReplay measures conversion and emission of one real paper fill.
*/
func BenchmarkPaperReplay(b *testing.B) {
	viper.Set("system.actor.buffer", 64)
	paper := NewPaper(context.Background(), NewSimulator())
	sub := paper.Subscribe("executions")
	trades := []any{map[string]any{
		"id": "PAPER-00026", "order_id": "PAPER-00025",
		"pair": "BTCUSD", "side": "buy", "volume": 0.0002,
		"price": 64129.9, "cost": 12.82598, "fee": 0.033347548,
		"status": "filled", "time": "2026-07-14T21:02:56Z",
	}}

	b.ReportAllocs()

	for b.Loop() {
		if err := paper.Replay(trades); err != nil {
			b.Fatal(err)
		}

		<-sub.Channel
	}
}
