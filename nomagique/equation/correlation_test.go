package equation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewCorrelation(t *testing.T) {
	Convey("Given the covariance normalization equation", t, func() {
		correlation := equation.NewCorrelation()

		Convey("successive runs preserve signed and asynchronous estimates", func() {
			for _, covariance := range []float64{2, -2, 0, 1} {
				input := tests.Record(map[string]any{
					"covariance": covariance, "left_energy": 1.0, "right_energy": 2.0,
				})
				output := tests.Drain(t, correlation, transport.NewIO(input))
				So(correlation.Error(), ShouldBeNil)
				So(output, ShouldHaveLength, 1)
				So(output[0], ShouldAlmostEqual, covariance/math.Sqrt2)
			}
		})

		Convey("zero energy leaves normalization undefined", func() {
			input := tests.Record(map[string]any{
				"covariance": 0.0, "left_energy": 0.0, "right_energy": 0.0,
			})
			output := tests.Drain(t, correlation, transport.NewIO(input))
			So(output, ShouldHaveLength, 1)
			So(math.IsNaN(output[0].(float64)), ShouldBeTrue)
			So(correlation.Error(), ShouldBeNil)
		})
	})
}

func BenchmarkNewCorrelation(b *testing.B) {
	correlation := equation.NewCorrelation()
	input := transport.NewIO(tests.Record(map[string]any{
		"covariance": 2.0, "left_energy": 1.0, "right_energy": 2.0,
	}))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		output := correlation.Next(input)

		if output == nil || core.To[float64](output) != 2/math.Sqrt(2) {
			b.Fatal("incorrect correlation output")
		}

		if correlation.Next(input) != nil {
			b.Fatal("correlation delivery did not end")
		}
	}

	if err := correlation.Error(); err != nil {
		b.Fatal(err)
	}
}
