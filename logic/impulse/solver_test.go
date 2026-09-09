package impulse

import (
	"errors"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func TestSolverStep(t *testing.T) {
	Convey("Signal and logic observations cross the grid stage with their own symbol identities", t, func() {
		solver := NewSolver()
		at := time.Unix(100, 0)
		measurement := data.NewMeasurement[float64]("flow", "BTC/USD", "cvd", at, at.Add(-time.Second))
		measurement.PutMetric(data.Metric[float64]{Label: "delta", Raw: 3})
		envelope := &types.Envelope{CVD: measurement,
			Categories: []types.Category{{Symbol: "ETH/USD", At: at, Type: types.VerticalIgnition, Strength: 4, Maturity: .75}},
		}
		So(solver.Step(envelope), ShouldEqual, envelope)
		So(solver.Error(), ShouldBeNil)
		So(len(envelope.Impulses), ShouldEqual, 2)
		So(len(solver.Rows), ShouldEqual, 2)
		So(len(solver.Columns), ShouldEqual, 6)
		So(solver.Latest["BTC/USD"].From, ShouldEqual, measurement.From)
		So(solver.Latest["BTC/USD"].Ready, ShouldBeFalse)

		Convey("A failed producer remains an explicit stage error", func() {
			measurement.Err = errors.New("producer failed")
			solver.Step(envelope)
			So(solver.Error(), ShouldNotBeNil)
		})
	})
}

func BenchmarkSolverStep(b *testing.B) {
	solver := NewSolver()
	at := time.Unix(100, 0)
	measurement := data.NewMeasurement[float64]("flow", "BTC/USD", "cvd", at, at)
	measurement.Metadata = map[string]float64{data.MetadataSupport: 100, data.MetadataMahalanobisSNR: 10}
	for index := range 412 {
		measurement.PutMetric(data.Metric[float64]{Label: strconv.Itoa(index), Raw: float64(index % 7)})
	}
	envelope := &types.Envelope{CVD: measurement}
	b.ReportAllocs()
	for b.Loop() {
		measurement.At = measurement.At.Add(time.Millisecond)
		for label, metric := range measurement.Metrics {
			metric.Raw = -metric.Raw
			measurement.Metrics[label] = metric
		}
		solver.Step(envelope)
		if err := solver.Error(); err != nil {
			b.Fatal(err)
		}
	}
}
