package trader

import (
	"testing"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLevel3Measure(t *testing.T) {
	Convey("Given level3 with a typed signal", t, func() {
		recording := &recordingSignal{}
		pool := testPool()
		level3 := NewLevel3(pool, &Signal{Level3: []types.Signal[any]{recording}}, testUIHub())
		raw := []byte(`[{"symbol":"BTC/USD","timestamp":"2026-07-04T12:00:00Z","checksum":291736120,"bids":[{"event":"add","order_id":"OQCLML-BW3P3-BUCMWZ","limit_price":43125.3,"order_qty":0.15,"timestamp":"2026-07-04T12:00:00Z"}]}]`)

		Convey("When level3 data is measured", func() {
			pushRing(level3.ring, raw)
			measurements, err := level3.Measure()

			Convey("It should measure each row through the signal", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				row := recording.rows[0].(kraken.Level3Data)
				So(row.Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func BenchmarkLevel3Measure(b *testing.B) {
	pool := testPool()
	level3 := NewLevel3(pool, &Signal{Level3: []types.Signal[any]{
		&benchmarkSignal{},
	}}, benchUIHub())
	raw := []byte(`[{"symbol":"BTC/USD","timestamp":"2026-07-04T12:00:00Z","checksum":291736120,"bids":[{"event":"add","order_id":"OQCLML-BW3P3-BUCMWZ","limit_price":43125.3,"order_qty":0.15,"timestamp":"2026-07-04T12:00:00Z"}]}]`)

	b.ReportAllocs()
	for b.Loop() {
		pushRing(level3.ring, raw)
		if _, err := level3.Measure(); err != nil {
			b.Fatal(err)
		}
	}
}
