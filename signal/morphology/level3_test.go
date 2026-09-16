package morphology

import (
	"github.com/theapemachine/symm/nomagique/runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

var baseTime = time.Unix(1_700_000_000, 0)

func row(
	symbol string,
	shapeDistance, shapeKS float64,
	concBid, concAsk float64,
	entBid, entAsk float64,
	at time.Time,
) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
		"book_shape_distance": data.NewMetric[float64]("book_shape_distance", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(shapeDistance),
		"book_shape_ks":       data.NewMetric[float64]("book_shape_ks", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(shapeKS),
		"concentration:bid":   data.NewMetric[float64]("concentration:bid", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(concBid),
		"concentration:ask":   data.NewMetric[float64]("concentration:ask", data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1).Write(concAsk),
		"entropy:bid":         data.NewMetric[float64]("entropy:bid", data.UnitNat, data.TimescaleInstantaneous, 0, 1).Write(entBid),
		"entropy:ask":         data.NewMetric[float64]("entropy:ask", data.UnitNat, data.TimescaleInstantaneous, 0, 1).Write(entAsk),
	})
	m.Label, m.At, m.From = symbol, at, at

	return m
}

func TestLevel3Step(t *testing.T) {
	Convey("Given a book morphology measuring instrument", t, func() {
		level3 := NewLevel3(t.Context())
		level3.Transition(runtime.READY)

		Convey("the first observation yields point metrics with no prior change", func() {
			measurement := level3.Step(row("BTC/USD", 0.05, 0.02, 0.4, 0.4, 1.2, 1.2, baseTime))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["book_shape_distance"].Raw, ShouldEqual, 0.05)
			So(measurement.Metrics["book_shape_ks"].Raw, ShouldEqual, 0.02)
			So(measurement.Metrics["concentration:bid"].Raw, ShouldEqual, 0.4)
			So(measurement.Metrics["concentration:ask"].Raw, ShouldEqual, 0.4)

			_, hasBaseline := measurement.Metrics["morphology_change_baseline"]
			So(hasBaseline, ShouldBeFalse)
			So(measurement.SNRDefined, ShouldBeFalse)
		})

		Convey("a moving book shape derives change, baselines, and a defined SNR", func() {
			var measurement *data.Measurement[float64]

			for step := range 12 {
				at := baseTime.Add(time.Duration(step) * time.Second)
				dist := 0.05 + float64(step*step)*0.005
				measurement = level3.Step(row("BTC/USD", dist, 0.02, 0.4, 0.4, 1.2, 1.2, at))
			}

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["morphology_change"].Raw, ShouldBeGreaterThan, 0)

			_, hasBaseline := measurement.Metrics["morphology_change_baseline"]
			So(hasBaseline, ShouldBeTrue)

			So(measurement.SNRDefined, ShouldBeTrue)
			So(measurement.SNR, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestLevel3Register(t *testing.T) {
	Convey("Given a Level3 entity", t, func() {
		level3 := NewLevel3(t.Context())
		level3.Transition(runtime.READY)
		schema := level3.Register()

		So(schema, ShouldNotBeNil)
		So(schema.Source, ShouldEqual, "morphology:level3")
		So(schema.Metrics, ShouldNotBeEmpty)

		expected := []string{
			"book_shape_distance",
			"book_shape_ks",
			"concentration:bid",
			"concentration:ask",
			"entropy:bid",
			"entropy:ask",
			"morphology_change",
			"morphology_change_baseline",
			"morphology_change_zscore",
		}

		for _, name := range expected {
			metric, ok := schema.Metrics[name]
			So(ok, ShouldBeTrue)
			So(metric.Label, ShouldEqual, name)
			So(metric.Raw, ShouldEqual, 0.0)
		}
	})
}

func TestLevel3StepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Level3{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(node.Step(measurement), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}

func TestLevel3StepUnrelatedPeer(t *testing.T) {
	Convey("An unrelated peer is not a fresh signal observation", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)
		measurement := entity.Register()
		peer := data.NewMeasurement[float64]("unrelated", nil)
		peer.Label = "BTC/USD"
		measurement.Peers = []*data.Measurement[float64]{peer}
		So(entity.Step(measurement), ShouldBeNil)
	})
}

func BenchmarkLevel3StepUnrelatedPeer(b *testing.B) {
	entity := NewLevel3(b.Context())
	entity.Transition(runtime.READY)
	measurement := entity.Register()
	peer := data.NewMeasurement[float64]("unrelated", nil)
	peer.Label = "BTC/USD"
	measurement.Peers = []*data.Measurement[float64]{peer}
	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		if entity.Step(measurement) != nil {
			b.Fatal("unrelated peer published a signal")
		}
	}
}
