package liquidity

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestGraphNext(t *testing.T) {
	Convey("Given independent bid, ask, and spread histories at irregular event times", t, func() {
		graph, projection := newLiquidityGraph(), liquidityProjection()
		var logTotals, residualTotals, timeResidualTotals [3]float64
		var timeTotal, timeSquaredTotal float64

		for index, quantity := range []float64{1, 2, 1.5, 3, 2.1, 4.2, 2.8, 5} {
			seconds := float64(index*index + index)
			askPrice := 101 + quantity
			askQuantity := 1 / quantity
			fields, err := transport.Evaluate(graph, transport.Values(GraphInput{
				BestBid: 100, BestAsk: askPrice, BidQty: quantity, AskQty: askQuantity,
				At: int64(seconds * float64(time.Second)),
			}))
			So(err, ShouldBeNil)
			measurement := projection.Project(fields)
			So(measurement.Err, ShouldBeNil)

			if index > 0 {
				timeTotal += seconds
				timeSquaredTotal += seconds * seconds
			}

			for coordinate, value := range []float64{100 * quantity, askPrice * askQuantity, (askPrice - 100) / ((askPrice + 100) / 2)} {
				logValue := math.Log(value)
				name := []string{"bid", "ask", "spread"}[coordinate]
				So(fields.Flags[name+"_has_prior"], ShouldEqual, index > 0)
				So(fields.Flags[name+"_slope_defined"], ShouldEqual, index >= 3)

				if index > 0 {
					residual := logValue - logTotals[coordinate]/float64(index)
					So(fields.Values[name+"_residual"], ShouldAlmostEqual, residual)
					residualTotals[coordinate] += residual
					timeResidualTotals[coordinate] += seconds * residual
				}

				if index >= 3 {
					count := float64(index)
					slope := (count*timeResidualTotals[coordinate] - timeTotal*residualTotals[coordinate]) /
						(count*timeSquaredTotal - timeTotal*timeTotal)
					So(fields.Values[name+"_slope"], ShouldAlmostEqual, slope)
				}

				logTotals[coordinate] += logValue
			}

			if index == 1 {
				So(measurement.Maturity, ShouldEqual, 0.5)
			}

			if index == 7 {
				So(measurement.SNRDefined, ShouldBeTrue)
			}
		}
	})
}

func BenchmarkGraphNext(b *testing.B) {
	graph := newLiquidityGraph()
	quantities := []float64{1, 2, 1.5, 3, 2.1, 4.2, 2.8, 5}
	step := 0
	b.ReportAllocs()

	for b.Loop() {
		quantity := quantities[step%len(quantities)]
		_, err := transport.Evaluate(graph, transport.Values(GraphInput{
			BestBid: 100, BestAsk: 101 + quantity, BidQty: quantity, AskQty: 1 / quantity,
			At: int64(step) * int64(time.Second),
		}))

		if err != nil {
			b.Fatal(err)
		}

		step++
	}
}
