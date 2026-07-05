package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLevel3Measure(testingTB *testing.T) {
	Convey("Given level3 with a typed signal", testingTB, func() {
		recording := &recordingSignal[kraken.Level3Data]{}
		level3 := NewLevel3([]types.Signal[kraken.Level3Data]{recording})
		message := kraken.Level3DataSlice{{
			Symbol:    "BTC/USD",
			Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
			Checksum:  291736120,
			Bids: []kraken.Level3Order{{
				Event:      "add",
				OrderID:    "OQCLML-BW3P3-BUCMWZ",
				LimitPrice: 43125.3,
				OrderQty:   0.15,
				Timestamp:  time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
			}},
		}}

		Convey("When level3 data is measured", func() {
			measurements, err := level3.Measure(message)

			Convey("It should measure each row through the signal", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldHaveLength, 1)
				So(recording.rows, ShouldHaveLength, 1)
				So(recording.rows[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func BenchmarkLevel3Measure(benchmarkTB *testing.B) {
	level3 := NewLevel3([]types.Signal[kraken.Level3Data]{
		&benchmarkSignal[kraken.Level3Data]{},
	})
	message := kraken.Level3DataSlice{{
		Symbol:    "BTC/USD",
		Timestamp: time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		Checksum:  291736120,
		Bids: []kraken.Level3Order{{
			Event:      "add",
			OrderID:    "OQCLML-BW3P3-BUCMWZ",
			LimitPrice: 43125.3,
			OrderQty:   0.15,
			Timestamp:  time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
		}},
	}}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		if _, err := level3.Measure(message); err != nil {
			benchmarkTB.Fatal(err)
		}
	}
}
