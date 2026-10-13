package correlation

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

func TestCorrelationSignalMetrics(t *testing.T) {
	Convey("Given a READY correlation signal", t, func() {
		now := time.Now()
		measurement := data.NewMeasurement(
			now.UnixNano(),
			"BTC/USD",
			"spot:trade",
			system.SeqIdx.Add(1),
			system.Tick.Add(1),
		)
		measurement.At = now
		measurement.From = now
		btcPrice := decimal.NewFromFloat64(50000)
		measurement.Write(
			data.NewExactMetric("price", btcPrice, data.UnitCurrency, data.TimescaleInstantaneous),
		)

		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)
		result := signal.Step(measurement)

		Convey("Then the initial single-asset tick produces no cross-asset metrics", func() {
			So(result, ShouldNotBeNil)

			metrics := make([]float64, 0)
			for metricEntry := range result.Read() {
				metrics = append(metrics, metricEntry.Metric.Raw())
			}

			// Cross-asset cohort covariance and recurrence require peer data.
			So(len(metrics), ShouldEqual, 0)
		})

		Convey("When peer observations arrive and prices move, cross-asset metrics are computed", func() {
			eth1 := data.NewMeasurement(
				now.Add(500*time.Millisecond).UnixNano(),
				"ETH/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			eth1.At = now.Add(500 * time.Millisecond)
			eth1.From = eth1.At
			eth1.Write(
				data.NewExactMetric("price", decimal.NewFromFloat64(3000), data.UnitCurrency, data.TimescaleInstantaneous),
			)
			So(signal.Step(eth1), ShouldNotBeNil)

			btc2 := data.NewMeasurement(
				now.Add(1500*time.Millisecond).UnixNano(),
				"BTC/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			btc2.At = now.Add(1500 * time.Millisecond)
			btc2.From = btc2.At
			btc2.Write(
				data.NewExactMetric("price", decimal.NewFromFloat64(50500), data.UnitCurrency, data.TimescaleInstantaneous),
			)
			So(signal.Step(btc2), ShouldNotBeNil)

			eth2 := data.NewMeasurement(
				now.Add(2000*time.Millisecond).UnixNano(),
				"ETH/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			eth2.At = now.Add(2000 * time.Millisecond)
			eth2.From = eth2.At
			eth2.Write(
				data.NewExactMetric("price", decimal.NewFromFloat64(3100), data.UnitCurrency, data.TimescaleInstantaneous),
			)
			So(signal.Step(eth2), ShouldNotBeNil)

			btc3 := data.NewMeasurement(
				now.Add(3000*time.Millisecond).UnixNano(),
				"BTC/USD",
				"spot:trade",
				system.SeqIdx.Add(1),
				system.Tick.Add(1),
			)
			btc3.At = now.Add(3000 * time.Millisecond)
			btc3.From = btc3.At
			btc3.Write(
				data.NewExactMetric("price", decimal.NewFromFloat64(51000), data.UnitCurrency, data.TimescaleInstantaneous),
			)

			movingBtcResult := signal.Step(btc3)
			So(movingBtcResult, ShouldNotBeNil)
			So(movingBtcResult.Error(), ShouldBeNil)

			movingKeys := make(map[string]bool)
			for metricEntry := range movingBtcResult.Read() {
				movingKeys[metricEntry.Key] = true
			}

			// Internal algorithmic intermediates are not leaked
			for _, internalKey := range []string{
				"last_price", "observation_count", "covariance", "covariance_standard_error",
				"signed_correlation", "absolute_correlation",
			} {
				So(movingKeys[internalKey], ShouldBeFalse)
			}
		})
	})
}

func TestCorrelationSignalWithoutPrice(t *testing.T) {
	Convey("A frame without a price yields no measurement instead of a panic", t, func() {
		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)

		frame := data.NewMeasurement(1, "BTC/USD", "spot:trade", system.SeqIdx.Add(1), system.Tick.Add(1))
		frame.At = time.Now()
		frame.From = frame.At

		So(func() { So(signal.Step(frame.Write()), ShouldBeNil) }, ShouldNotPanic)
	})
}

func TestCorrelationReferencePeerHasMostOverlap(t *testing.T) {
	Convey("The single-pair metrics describe the peer with the most overlapping data", t, func() {
		signal := NewSignal(t.Context())
		signal.Transition(runtime.READY)
		origin := time.Unix(1700000000, 0).UTC()
		random := rand.New(rand.NewSource(17))
		walk := map[string]float64{"AAA/USD": 100, "BTC/USD": 50000, "ZZZ/USD": 10}

		step := func(symbol string, offset time.Duration) *data.Measurement {
			walk[symbol] *= math.Exp(random.NormFloat64() * 0.01)
			frame := data.NewMeasurement(origin.Add(offset).UnixNano(), symbol, "spot:trade", system.SeqIdx.Add(1), system.Tick.Add(1))
			frame.At = origin.Add(offset)
			frame.From = frame.At
			frame.Write(data.NewExactMetric("price", decimal.NewFromFloat64(walk[symbol]), data.UnitCurrency, data.TimescaleInstantaneous))
			return signal.Step(frame)
		}

		var last *data.Measurement

		for second := 0; second <= 12; second++ {
			at := time.Duration(second) * time.Second

			if second%5 == 0 {
				step("ZZZ/USD", at+100*time.Millisecond)
			}

			step("AAA/USD", at+500*time.Millisecond)
			last = step("BTC/USD", at+900*time.Millisecond)
		}

		So(last, ShouldNotBeNil)
		So(last.Error(), ShouldBeNil)
		So(last.Meta(data.MetadataReferencePeer), ShouldEqual, "AAA/USD")
	})
}
