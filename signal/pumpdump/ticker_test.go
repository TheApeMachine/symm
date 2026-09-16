package pumpdump

import (
	"github.com/theapemachine/symm/nomagique/runtime"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func spotTicker(symbol string, bid float64, ask float64, at time.Time) *data.Measurement[float64] {
	m := data.NewMeasurement[float64]("websocket", map[string]data.Metric[float64]{
		"best_bid": data.NewMetric[float64]("best_bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(bid),
		"best_ask": data.NewMetric[float64]("best_ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(ask),
	})
	m.Label, m.At, m.From = symbol, at, at

	return m
}

func TestTickerStep(t *testing.T) {
	Convey("Given a valid executable touch", t, func() {
		entity := NewTicker(t.Context())
		entity.Transition(runtime.READY)
		at := time.Unix(1_700_000_000, 0)

		Convey("the first data point yields the touch and its own baseline", func() {
			measurement := entity.Step(spotTicker("BTC/USD", 99, 101, at))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["best_bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["midpoint"].Raw, ShouldEqual, 100.0)
			So(measurement.Metrics["spread"].Raw, ShouldEqual, 2.0)
			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.02, 1e-12)

			// The baseline of one value is the value itself: ratio one.
			So(measurement.Metrics["relative_spread_baseline"].Raw, ShouldAlmostEqual, 0.02, 1e-9)
			So(measurement.Metrics["spread_ratio"].Raw, ShouldAlmostEqual, 1.0, 1e-9)

			// No prior baseline exists yet, so the divergence and z-score are
			// not estimable on the first observation.
			_, hasDivergence := measurement.Metrics["spread_divergence"]
			So(hasDivergence, ShouldBeFalse)
			_, hasZ := measurement.Metrics["spread_zscore"]
			So(hasZ, ShouldBeFalse)

			// One retained estimator sample is still immature.
			So(measurement.Maturity, ShouldEqual, 0.0)
		})

		Convey("a narrower follow-up touch is measured below its baseline", func() {
			entity.Step(spotTicker("BTC/USD", 99, 101, at))
			measurement := entity.Step(spotTicker("BTC/USD", 99.5, 100.5, at.Add(10*time.Second)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)

			So(measurement.Metrics["relative_spread"].Raw, ShouldAlmostEqual, 0.01, 1e-12)
			So(measurement.Metrics["spread_ratio"].Raw, ShouldBeLessThan, 1.0)
			So(measurement.Metrics["spread_divergence"].Raw, ShouldAlmostEqual, math.Log(0.5), 1e-12)
			So(measurement.Metrics["spread_zscore"].Raw, ShouldAlmostEqual, -math.Sqrt(2.0), 1e-9)
		})
	})

	Convey("Given a crossed touch snapshot", t, func() {
		entity := NewTicker(t.Context())
		entity.Transition(runtime.READY)

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := entity.Step(spotTicker("BTC/USD", 101, 99, time.Unix(1_700_000_000, 0)))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}

func TestTickerRegister(t *testing.T) {
	Convey("Given a Ticker entity", t, func() {
		entity := NewTicker(t.Context())
		entity.Transition(runtime.READY)
		schema := entity.Register()

		So(schema, ShouldNotBeNil)
		So(schema.Source, ShouldEqual, "pumpdump:ticker")
		So(schema.Metrics, ShouldNotBeEmpty)

		expected := []string{
			"best_bid",
			"best_ask",
			"midpoint",
			"spread",
			"relative_spread",
			"relative_spread_baseline",
			"spread_ratio",
			"spread_divergence",
			"spread_zscore",
		}

		for _, name := range expected {
			metric, ok := schema.Metrics[name]
			So(ok, ShouldBeTrue)
			So(metric.Label, ShouldEqual, name)
			So(metric.Raw, ShouldEqual, 0.0)
		}
	})
}

func TestTickerStepReadiness(t *testing.T) {
	Convey("An inactive pipeline node drops input before touching processing state", t, func() {
		node := &Ticker{System: runtime.NewSystem(t.Context(), "readiness-test")}
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 7}
		for _, stage := range []runtime.Stage{runtime.INIT, runtime.WAITING, runtime.ERROR, runtime.FATAL} {
			node.Transition(stage)
			So(node.Step(measurement), ShouldEqual, measurement)
			So(node.Status(), ShouldEqual, stage)
			So(measurement.SeqIdx, ShouldEqual, 7)
		}
	})
}

func TestTickerStepUnrelatedPeer(t *testing.T) {
	Convey("An unrelated peer does not publish registration values as a fresh observation", t, func() {
		entity := NewTicker(t.Context())
		entity.Transition(runtime.READY)
		measurement := entity.Register()
		peer := data.NewMeasurement[float64]("unrelated", nil)
		peer.Label = "BTC/USD"
		measurement.Peers = []*data.Measurement[float64]{peer}
		So(entity.Step(measurement), ShouldBeNil)
		So(measurement.Label, ShouldBeEmpty)
	})
}

func BenchmarkTickerStep(b *testing.B) {
	entity := NewTicker(b.Context())
	entity.Transition(runtime.READY)
	peer := spotTicker("BTC/USD", 99, 101, time.Unix(1, 0))
	measurement := entity.Register()
	measurement.Peers = []*data.Measurement[float64]{peer}
	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		peer.At = peer.At.Add(time.Second)
		result := entity.Step(measurement)

		if result == nil || result.Err != nil {
			b.Fatal("valid peer was not processed")
		}
	}
}
