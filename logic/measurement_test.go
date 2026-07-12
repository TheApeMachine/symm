package logic

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

func TestMeasurementAnalyzerIngest(t *testing.T) {
	Convey("Given typed Hawkes evidence and an unmigrated signal value", t, func() {
		thesis := strategy.NewThesis()
		analyzer := NewMeasurementAnalyzer(thesis)
		typed := composerMeasurement("BTC/USD", time.Unix(1, 0), types.SideBuy)
		legacy := &types.Measurement{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
			Metrics: map[string]float64{
				"net": 2,
			},
		}

		Convey("When the existing trading loop submits one batch", func() {
			err := analyzer.Ingest([]*types.Measurement{typed, legacy})

			Convey("Then typed history is preserved and legacy evidence remains live", func() {
				So(err, ShouldBeNil)
				epochs := thesis.Epochs("BTC/USD")
				So(epochs, ShouldHaveLength, 1)
				So(epochs[0].Measurements, ShouldHaveLength, 1)
				So(epochs[0].Measurements[0].Metric, ShouldEqual, typed.Metric)

				current, ok := thesis.Evidence("BTC/USD", string(types.SourceCVD))
				So(ok, ShouldBeTrue)
				So(current, ShouldEqual, legacy)
			})
		})
	})

	Convey("Given a legacy value followed by invalid typed evidence", t, func() {
		thesis := strategy.NewThesis()
		analyzer := NewMeasurementAnalyzer(thesis)
		legacy := &types.Measurement{
			Source: types.SourceCVD,
			Symbol: "BTC/USD",
		}
		invalid := composerMeasurement("BTC/USD", time.Unix(1, 0), types.SideBuy)
		invalid.Unit = ""

		Convey("When the complete batch is validated", func() {
			err := analyzer.Ingest([]*types.Measurement{legacy, invalid})

			Convey("Then no partial Thesis update survives the failed precondition", func() {
				So(err, ShouldNotBeNil)
				So(thesis.Symbols(), ShouldBeEmpty)
				So(thesis.Epochs("BTC/USD"), ShouldBeEmpty)
			})
		})
	})
}

func BenchmarkMeasurementAnalyzerIngest(b *testing.B) {
	const symbols = 1455
	measurements := make([]*types.Measurement, symbols)

	for index := range symbols {
		measurements[index] = composerMeasurement(
			fmt.Sprintf("ASSET-%04d/USD", index),
			time.Unix(1, 0),
			types.SideBuy,
		)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		analyzer := NewMeasurementAnalyzer(strategy.NewThesis())

		if err := analyzer.Ingest(measurements); err != nil {
			b.Fatal(err)
		}
	}
}
