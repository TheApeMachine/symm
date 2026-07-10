package trader

import (
	"testing"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTradeMeasure(t *testing.T) {
	Convey("Given a trade with a typed signal", t, func() {
		pool := testPool()
		recording := &recordingSignal{}
		trade := NewTrade(pool, &Signal{Trade: []types.Signal[any]{recording}}, testUIHub())
		raw := []byte(`[{"symbol":"MATIC/USD","side":"buy","price":0.5147,"qty":6423.46326,"ord_type":"limit","trade_id":4665846,"timestamp":"2026-07-04T12:00:00Z"}]`)

		Convey("When trade data is measured", func() {
			pushRing(trade.ring, raw)
			measurements, err := trade.Measure()

			Convey("It should measure each row through the signal", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				row := recording.rows[0].(kraken.TradeData)
				So(row.Symbol, ShouldEqual, "MATIC/USD")
			})
		})
	})
}

func BenchmarkTradeMeasure(b *testing.B) {
	pool := testPool()
	trade := NewTrade(pool, &Signal{Trade: []types.Signal[any]{
		&benchmarkSignal{},
	}}, benchUIHub())
	raw := []byte(`[{"symbol":"MATIC/USD","side":"buy","price":0.5147,"qty":6423.46326,"ord_type":"limit","trade_id":4665846,"timestamp":"2026-07-04T12:00:00Z"}]`)

	b.ReportAllocs()
	for b.Loop() {
		pushRing(trade.ring, raw)
		if _, err := trade.Measure(); err != nil {
			b.Fatal(err)
		}
	}
}
