package pumpdump

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/nomagique/data/sequence"

	"github.com/theapemachine/symm/nomagique/runtime"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func pumpdumpTouch(symbol string, bid float64, ask float64, at time.Time) *data.Measurement[float64] {
	metrics := make(map[string]data.Metric[float64])

	if bid > 0 {
		metrics["best_bid"] = data.NewMetric[float64]("best_bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(bid)
	}

	if ask > 0 {
		metrics["best_ask"] = data.NewMetric[float64]("best_ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(ask)
	}

	m := data.NewMeasurement[float64]("websocket", metrics)
	m.Label, m.At, m.From = symbol, at, at

	return m
}

func TestLevel3Next(t *testing.T) {
	Convey("Given a message with an executable touch", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)
		at := time.Unix(1_700_000_000, 0)

		Convey("Step derives the touch from the message's own orders", func() {
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](pumpdumpTouch("BTC/USD", 99, 101, at))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)

			So(measurement.Maturity, ShouldEqual, 1.0)
		})
	})

	Convey("Given a symbol whose book has never shown both sides", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)
		at := time.Unix(1_700_000_000, 0)

		Convey("Step yields no measurement rather than an error", func() {
			So(sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](pumpdumpTouch("MISSING", 0, 0, at)))), ShouldBeNil)
		})

		Convey("A one-sided message alone still yields no measurement", func() {
			So(sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](pumpdumpTouch("ONESIDED", 99, 0, at)))), ShouldBeNil)
		})
	})

	Convey("Given a symbol that has seen both sides across separate messages", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)
		at := time.Unix(1_700_000_000, 0)

		// First observation carries bid only
		So(sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](pumpdumpTouch("BTC/USD", 99, 0, at)))), ShouldBeNil)

		Convey("A later one-sided update borrows the retained opposite side", func() {
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](pumpdumpTouch("BTC/USD", 0, 101, at.Add(time.Second)))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
		})
	})
}

func TestLevel3Register(t *testing.T) {
	Convey("Given a Level3 entity", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)
		schema := entity.Register()

		So(schema, ShouldNotBeNil)
		So(schema.Source, ShouldEqual, "pumpdump:level3")
		So(schema.Metrics, ShouldNotBeEmpty)

		expected := []string{
			"best_bid",
			"best_ask",
			"midpoint",
			"spread",
			"relative_spread",
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
			So(sequence.Read[*data.Measurement[float64]](node.Next(sequence.NewValue[*data.Measurement[float64]](measurement))), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}

func TestLevel3StepUnrelatedPeer(t *testing.T) {
	Convey("An unrelated peer does not publish registration values as a fresh observation", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)
		measurement := entity.Register()
		peer := data.NewMeasurement[float64]("unrelated", nil)
		peer.Label = "BTC/USD"
		measurement.Peers = []*data.Measurement[float64]{peer}
		So(sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](measurement))), ShouldBeNil)
		So(measurement.Label, ShouldBeEmpty)
	})
}

func BenchmarkLevel3Next(b *testing.B) {
	entity := NewLevel3(b.Context())
	entity.Transition(runtime.READY)
	peer := pumpdumpTouch("BTC/USD", 99, 101, time.Unix(1, 0))
	measurement := entity.Register()
	measurement.Peers = []*data.Measurement[float64]{peer}
	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		peer.At = peer.At.Add(time.Second)
		result := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](measurement)))

		if result == nil || result.Err != nil {
			b.Fatal("valid peer was not processed")
		}
	}
}
