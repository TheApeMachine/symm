package toxicity

import (
	"math"
	"testing"
	"time"

	"github.com/theapemachine/symm/nomagique/data/sequence"

	"github.com/theapemachine/symm/nomagique/runtime"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/market"
)

func toxicityTouch(symbol string, at time.Time, bidPrice, bidQty, askPrice, askQty float64) *data.Measurement[float64] {
	m := data.NewMeasurement("websocket", map[string]data.Metric[float64]{
		"best_price:bid":     data.NewMetric[float64]("best_price:bid", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(bidPrice),
		"best_price:ask":     data.NewMetric[float64]("best_price:ask", data.UnitRate, data.TimescaleInstantaneous, 0, 1).Write(askPrice),
		"touch_quantity:bid": data.NewMetric[float64]("touch_quantity:bid", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(bidQty),
		"touch_quantity:ask": data.NewMetric[float64]("touch_quantity:ask", data.UnitCount, data.TimescaleInstantaneous, 0, 1).Write(askQty),
	})
	m.Label, m.At, m.From = symbol, at, at

	return m
}

func TestLevel3Next(t *testing.T) {
	Convey("Given a sequence of touch observations", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)

		Convey("the first observation anchors the previous touch", func() {
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](toxicityTouch("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics, ShouldNotBeEmpty)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_price:ask"].Raw, ShouldEqual, 101.0)
			So(measurement.Metrics["touch_quantity:bid"].Raw, ShouldEqual, 10.0)
			So(measurement.Metrics["touch_quantity:ask"].Raw, ShouldEqual, 12.0)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["unfilled_residual_quantity:bid"].Raw, ShouldEqual, 10.0)

			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})

		Convey("a later observation attributes a bid retreat", func() {
			first := time.Unix(1_700_000_000, 0)
			second := time.Unix(1_700_000_001, 0)

			sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](toxicityTouch("BTC/USD", first, 99, 10, 101, 12))))
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](toxicityTouch("BTC/USD", second, 98, 5, 101, 12))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["previous_best_price:bid"].Raw, ShouldEqual, 99.0)
			So(measurement.Metrics["best_price:bid"].Raw, ShouldEqual, 98.0)
			So(measurement.Metrics["touch_price_log_change:bid"].Raw, ShouldAlmostEqual, math.Log(98.0/99.0), 1e-12)
			So(measurement.Metrics["retreated_quantity:bid"].Raw, ShouldEqual, 10.0)
			So(measurement.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["net_replenished_quantity:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["retreat_fraction:bid"].Raw, ShouldAlmostEqual, 1.0, 1e-12)
			So(measurement.Metrics["net_withdrawal_fraction:bid"].Raw, ShouldEqual, 0.0)
			So(measurement.Metrics["retreat_rate:bid"].Raw, ShouldAlmostEqual, 10.0, 1e-12)
		})

		Convey("a later observation attributes an unchanged-touch withdrawal", func() {
			sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](toxicityTouch("BTC/USD", time.Unix(1_700_000_000, 0), 99, 10, 101, 12))))
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](toxicityTouch("BTC/USD", time.Unix(1_700_000_001, 0), 99, 4, 101, 12))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Metrics["net_withdrawn_quantity:bid"].Raw, ShouldEqual, 6.0)
			So(measurement.Metrics["net_withdrawal_fraction:bid"].Raw, ShouldAlmostEqual, 0.6, 1e-12)
		})
	})

	Convey("A rejected peer retains its own identity between valid market legs", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)
		measurement := entity.Register()
		tape := market.NewOpportunityTape("BTC/USD", time.Unix(1_700_000_000, 0), 2)

		for index, step := range tape.Steps {
			// One dollar spread and fixed quantities define this quote fixture.
			quote := toxicityTouch(tape.Symbol, step.EventTime, step.ExecutableBid, 10, step.ExecutableBid+1, 12)
			quote.SeqIdx = int64(index*2 + 1)
			quote.Timestamp = quote.At.UnixNano()
			quote.Provenance["channel"] = "level3"
			measurement.Err = nil // Consumer clears the prior observation's error.
			measurement.Peers = []*data.Measurement[float64]{quote}
			So(sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](measurement))), ShouldEqual, measurement)
			So(measurement.Err, ShouldBeNil)
			So(measurement.Label, ShouldEqual, tape.Symbol)

			// This locked futures quote is the recorded failing input.
			rejected := toxicityTouch("FARTCOIN/USD", step.EventTime.Add(time.Nanosecond), 0.1351, 10, 0.1351, 12)
			rejected.SeqIdx = quote.SeqIdx + 1
			rejected.Timestamp = rejected.At.UnixNano()
			rejected.Provenance["channel"] = "futures.ticker"
			measurement.Peers[0] = rejected
			So(sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](measurement))), ShouldEqual, measurement)
			So(measurement.Err, ShouldNotBeNil)
			So(measurement.Label, ShouldEqual, rejected.Label)
			So(measurement.At, ShouldEqual, rejected.At)
			So(measurement.From, ShouldEqual, rejected.From)
			So(measurement.Timestamp, ShouldEqual, rejected.Timestamp)
			So(measurement.SeqIdx, ShouldEqual, rejected.SeqIdx)
			So(measurement.Provenance["channel"], ShouldEqual, "futures.ticker")
		}
	})

	Convey("Given a crossed touch", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)

		Convey("the measurement carries the pipeline rejection in its Err field", func() {
			measurement := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](toxicityTouch("BTC/USD", time.Unix(1_700_000_000, 0), 101, 10, 99, 12))))

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldNotBeNil)
		})
	})
}

func TestLevel3Register(t *testing.T) {
	Convey("Given a Level3 entity", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)
		schema := entity.Register()

		So(schema, ShouldNotBeNil)
		So(schema.Source, ShouldEqual, "toxicity:level3")
		So(schema.Metrics, ShouldNotBeEmpty)

		expected := []string{
			"best_price:bid",
			"best_price:ask",
			"touch_quantity:bid",
			"touch_quantity:ask",
			"unfilled_residual_quantity:bid",
			"unfilled_residual_quantity:ask",
			"previous_best_price:bid",
			"previous_best_price:ask",
			"touch_price_log_change:bid",
			"touch_price_log_change:ask",
			"retreated_quantity:bid",
			"net_withdrawn_quantity:bid",
			"net_replenished_quantity:bid",
			"retreat_fraction:bid",
			"net_withdrawal_fraction:bid",
			"retreat_rate:bid",
			"retreated_quantity:ask",
			"net_withdrawn_quantity:ask",
			"net_replenished_quantity:ask",
			"retreat_fraction:ask",
			"net_withdrawal_fraction:ask",
			"retreat_rate:ask",
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
	Convey("An unrelated peer is not a fresh signal observation", t, func() {
		entity := NewLevel3(t.Context())
		entity.Transition(runtime.READY)
		measurement := entity.Register()
		peer := data.NewMeasurement[float64]("unrelated", nil)
		peer.Label = "BTC/USD"
		measurement.Peers = []*data.Measurement[float64]{peer}
		So(sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](measurement))), ShouldBeNil)
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
		if sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](measurement))) != nil {
			b.Fatal("unrelated peer published a signal")
		}
	}
}

func BenchmarkLevel3Next(b *testing.B) {
	entity := NewLevel3(b.Context())
	entity.Transition(runtime.READY)
	measurement := entity.Register()
	tape := market.NewOpportunityTape("BTC/USD", time.Unix(1_700_000_000, 0), 2)
	quotes := make([]*data.Measurement[float64], len(tape.Steps))

	for index, step := range tape.Steps {
		quotes[index] = toxicityTouch(tape.Symbol, step.EventTime, step.ExecutableBid, 10, step.ExecutableBid+1, 12)
		quotes[index].Provenance["channel"] = "level3"
	}

	measurement.Peers = make([]*data.Measurement[float64], 1)
	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		quote := quotes[index%len(quotes)]
		quote.SeqIdx = int64(index + 1)
		quote.At = tape.Steps[0].EventTime.Add(time.Duration(index) * time.Millisecond)
		measurement.Peers[0] = quote

		if result := sequence.Read[*data.Measurement[float64]](entity.Next(sequence.NewValue[*data.Measurement[float64]](measurement))); result == nil || result.Err != nil {
			b.Fatal("valid quote rejected", measurement.Err)
		}
	}
}
