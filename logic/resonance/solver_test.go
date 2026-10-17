package resonance

import (
	"context"
	"fmt"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
)

/*
symbolTick is one symbol of a stored tick carrying the given Features'
values (by source), as broker.ReadTick hands it over.
*/
func symbolTick(symbol string, values map[string]float64, touch bool) *broker.SymbolTick {
	out := &broker.SymbolTick{Symbol: symbol}

	if touch {
		out.Price = broker.PriceLayer{Symbol: symbol, Bid: decimal.NewFromFloat64(99.9), Ask: decimal.NewFromFloat64(100.1)}
	}

	for _, feature := range Features {
		if val, ok := values[feature.Source]; ok {
			out.Signals = append(out.Signals, data.Signal{
				Source: feature.Source, Symbol: symbol, Labels: []string{feature.Label}, Values: []float64{val},
			})
		}
	}

	return out
}

func stored(tick uint64) *data.Value {
	value := data.NewValue(data.TickOrigin, data.TickRole, data.TickScope)
	value.Epoch, value.Tick, value.Timestamp = 7, tick, tick*1e9
	value.FormatPrefix()
	return value
}

func every(val float64) map[string]float64 {
	values := make(map[string]float64, len(Features))

	for idx, feature := range Features {
		values[feature.Source] = val + float64(idx)/10
	}

	return values
}

func TestSolver(t *testing.T) {
	Convey("Given a resonance solver", t, func() {
		solver, err := NewSolver(context.Background(), 0.03)
		So(err, ShouldBeNil)
		defer solver.Close()

		Convey("A pace that is not positive is refused", func() {
			_, err := NewSolver(context.Background(), 0)
			So(err, ShouldNotBeNil)
		})

		Convey("A step over every pinned feature reads surprise, energy and every coder layer", func() {
			reading, err := solver.Step(stored(1), symbolTick("BTC/USD", every(1), true))
			So(err, ShouldBeNil)
			So(reading.Origin, ShouldEqual, ReadingOrigin)
			So(reading.Scope, ShouldEqual, "BTC/USD")
			So(reading.Tick, ShouldEqual, 1)

			surprise, err := ReadSurprise(reading)
			So(err, ShouldBeNil)
			So(surprise, ShouldBeGreaterThanOrEqualTo, 0)

			metrics := data.Metrics(reading)
			So(metrics, ShouldContainKey, "energy")

			for layer, rows := range coderArch(len(Features)) {
				So(metrics, ShouldContainKey, fmt.Sprintf("layer_%d_error", layer))
				So(metrics, ShouldContainKey, fmt.Sprintf("layer_%d_state_%d", layer, rows-1))
				So(metrics, ShouldNotContainKey, fmt.Sprintf("layer_%d_state_%d", layer, rows))
			}
		})

		Convey("Sources a tick does not carry, and an absent touch, still step", func() {
			_, err := solver.Step(stored(1), symbolTick("BTC/USD", map[string]float64{"cvd": 0.1}, false))
			So(err, ShouldBeNil)

			reading, err := solver.Step(stored(2), symbolTick("BTC/USD", map[string]float64{"toxicity": 0.2}, true))
			So(err, ShouldBeNil)
			_, err = ReadSurprise(reading)
			So(err, ShouldBeNil)
		})

		Convey("A carried source without its pinned label is an error naming it", func() {
			tick := symbolTick("BTC/USD", every(1), true)
			tick.Signals[0].Labels = []string{"covariance"}

			_, err := solver.Step(stored(1), tick)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, Features[0].Label)
		})

		Convey("A source with no pinned feature is an error", func() {
			tick := symbolTick("BTC/USD", every(1), true)
			tick.Signals = append(tick.Signals, data.Signal{Source: "derivatives", Symbol: "BTC/USD", Labels: []string{"basis"}, Values: []float64{1}})

			_, err := solver.Step(stored(1), tick)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "derivatives")
		})

		Convey("Two solvers fed the same ticks read the same surprise", func() {
			other, err := NewSolver(context.Background(), 0.03)
			So(err, ShouldBeNil)
			defer other.Close()

			for tick := uint64(1); tick <= 20; tick++ {
				values := every(float64(tick % 5))
				left, err := solver.Step(stored(tick), symbolTick("ETH/USD", values, true))
				So(err, ShouldBeNil)
				right, err := other.Step(stored(tick), symbolTick("ETH/USD", values, true))
				So(err, ShouldBeNil)

				a, _ := ReadSurprise(left)
				b, _ := ReadSurprise(right)
				So(a, ShouldEqual, b)
			}
		})
	})
}

func TestPublishReturnsRejectsMisshapedReadings(t *testing.T) {
	Convey("Given a coder reading that disagrees with its shape", t, func() {
		Convey("A layer reading that disagrees with the architecture is an error", func() {
			var out [12][]float64
			out[0] = make([]float64, 12)
			out[7] = make([]float64, 2+2*3+1)

			_, err := publishReturns(data.New(), out, []int{3})
			So(err, ShouldNotBeNil)
		})

		Convey("A summary too short to hold surprise is an error, not a zero", func() {
			var out [12][]float64
			out[0] = make([]float64, 2)

			_, err := publishReturns(data.New(), out, []int{3})
			So(err, ShouldNotBeNil)
		})
	})
}
