package trader

import (
	"testing"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOHLCMeasure(t *testing.T) {
	Convey("Given OHLC with a typed signal", t, func() {
		recording := &recordingSignal{}
		pool := testPool()
		ohlc := NewOHLC(pool, &Signal{OHLC: []types.Signal[any]{recording}}, testUIHub())
		raw := []byte(`[{"symbol":"ALGO/USD","open":0.09875,"high":0.0988,"low":0.09875,"close":0.09875,"trades":13,"volume":16255.46368,"vwap":0.09879,"interval_begin":"2026-07-04T11:55:00Z","interval":5,"timestamp":"2026-07-04T12:00:00Z"}]`)

		Convey("When OHLC data is measured", func() {
			pushRing(ohlc.ring, raw)
			measurements, err := ohlc.Measure()

			Convey("It should measure each row through the signal", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				row := recording.rows[0].(kraken.OHLCData)
				So(row.Symbol, ShouldEqual, "ALGO/USD")
			})
		})
	})
}

func BenchmarkOHLCMeasure(b *testing.B) {
	pool := testPool()
	ohlc := NewOHLC(pool, &Signal{OHLC: []types.Signal[any]{
		&benchmarkSignal{},
	}}, benchUIHub())
	raw := []byte(`[{"symbol":"ALGO/USD","open":0.09875,"high":0.0988,"low":0.09875,"close":0.09875,"trades":13,"volume":16255.46368,"vwap":0.09879,"interval_begin":"2026-07-04T11:55:00Z","interval":5,"timestamp":"2026-07-04T12:00:00Z"}]`)

	b.ReportAllocs()
	for b.Loop() {
		pushRing(ohlc.ring, raw)
		if _, err := ohlc.Measure(); err != nil {
			b.Fatal(err)
		}
	}
}
