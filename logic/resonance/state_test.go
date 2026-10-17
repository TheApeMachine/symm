package resonance

import (
	"context"
	"math"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

/*
varied is a tick of every feature that keeps moving (so the standardizers,
the pace and every horizon of the head all learn), with a moving touch.
*/
func varied(symbol string, tick uint64) *symbolTickArgs {
	values := make(map[string]float64, len(Features))

	for idx, feature := range Features {
		values[feature.Source] = math.Sin(float64(tick)*0.37+float64(idx)) + 0.1*math.Cos(float64(tick*tick%17))
	}

	mid := 100 + math.Sin(float64(tick)*0.11)*2
	return &symbolTickArgs{symbol: symbol, values: values, mid: mid}
}

type symbolTickArgs struct {
	symbol string
	values map[string]float64
	mid    float64
}

func stepVaried(solver *Solver, symbol string, tick uint64) ([][]byte, error) {
	args := varied(symbol, tick)
	in := symbolTick(args.symbol, args.values, true)
	in.Price.Bid = decimal.NewFromFloat64(args.mid - 0.05)
	in.Price.Ask = decimal.NewFromFloat64(args.mid + 0.05)

	reading, err := solver.Step(stored(tick), in)

	if err != nil {
		return nil, err
	}

	return reading.Layers, nil
}

func TestSolverState(t *testing.T) {
	Convey("Given a solver that stepped a symbol for a while", t, func() {
		trained, err := NewSolver(context.Background(), 0.03)
		So(err, ShouldBeNil)
		defer trained.Close()

		for tick := uint64(1); tick <= 60; tick++ {
			_, err := stepVaried(trained, "BTC/USD", tick)
			So(err, ShouldBeNil)
		}

		So(trained.Symbols(), ShouldResemble, []string{"BTC/USD"})

		blob, dropped, err := trained.SaveSymbol("BTC/USD")
		So(err, ShouldBeNil)
		So(dropped, ShouldBeGreaterThan, 0)

		Convey("a solver loading its state continues exactly as it does (its pending predictions dropped)", func() {
			loaded, err := NewSolver(context.Background(), 0.03)
			So(err, ShouldBeNil)
			defer loaded.Close()
			So(loaded.LoadSymbol("BTC/USD", blob), ShouldBeNil)

			for tick := uint64(61); tick <= 120; tick++ {
				want, err := stepVaried(trained, "BTC/USD", tick)
				So(err, ShouldBeNil)
				got, err := stepVaried(loaded, "BTC/USD", tick)
				So(err, ShouldBeNil)
				So(got, ShouldResemble, want)
			}

			Convey("and its own saved state is the same again", func() {
				left, _, err := trained.SaveSymbol("BTC/USD")
				So(err, ShouldBeNil)
				right, _, err := loaded.SaveSymbol("BTC/USD")
				So(err, ShouldBeNil)
				So(right, ShouldResemble, left)
			})
		})

		Convey("a cold solver reads differently: the state is what carries the learning", func() {
			cold, err := NewSolver(context.Background(), 0.03)
			So(err, ShouldBeNil)
			defer cold.Close()

			want, err := stepVaried(trained, "BTC/USD", 61)
			So(err, ShouldBeNil)
			got, err := stepVaried(cold, "BTC/USD", 61)
			So(err, ShouldBeNil)
			So(got, ShouldNotResemble, want)
		})

		Convey("a state is refused by a symbol already stepped, when malformed, or at another pace", func() {
			So(trained.LoadSymbol("BTC/USD", blob), ShouldNotBeNil)

			fresh, err := NewSolver(context.Background(), 0.03)
			So(err, ShouldBeNil)
			defer fresh.Close()
			So(fresh.LoadSymbol("BTC/USD", blob[:len(blob)/2]), ShouldNotBeNil)
			So(fresh.Symbols(), ShouldBeEmpty)

			other, err := NewSolver(context.Background(), 0.05)
			So(err, ShouldBeNil)
			defer other.Close()
			err = other.LoadSymbol("BTC/USD", blob)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "pace state was configured")
			So(other.Symbols(), ShouldBeEmpty)
		})

		Convey("an unknown symbol has nothing to save", func() {
			_, _, err := trained.SaveSymbol("ETH/USD")
			So(err, ShouldNotBeNil)
		})
	})
}
