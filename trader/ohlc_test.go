package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOHLCMeasure(testingTB *testing.T) {
	Convey("Given OHLC with a typed signal", testingTB, func() {
		recording := &recordingSignal{}
		ohlc := NewOHLC([]types.Signal[any]{recording})
		message := kraken.OHLCDataSlice{{
			Symbol:        "ALGO/USD",
			Open:          0.09875,
			High:          0.0988,
			Low:           0.09875,
			Close:         0.09875,
			Trades:        13,
			Volume:        16255.46368,
			Vwap:          0.09879,
			IntervalBegin: time.Date(2026, 7, 4, 11, 55, 0, 0, time.UTC),
			Interval:      5,
			Timestamp:     time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}}

		Convey("When OHLC data is measured", func() {
			measurements, err := ohlc.Measure(message)

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

func BenchmarkOHLCMeasure(benchmarkTB *testing.B) {
	ohlc := NewOHLC([]types.Signal[any]{
		&benchmarkSignal{},
	})
	message := kraken.OHLCDataSlice{{
		Symbol:        "ALGO/USD",
		Open:          0.09875,
		High:          0.0988,
		Low:           0.09875,
		Close:         0.09875,
		Trades:        13,
		Volume:        16255.46368,
		Vwap:          0.09879,
		IntervalBegin: time.Date(2026, 7, 4, 11, 55, 0, 0, time.UTC),
		Interval:      5,
		Timestamp:     time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	}}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		if _, err := ohlc.Measure(message); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}
