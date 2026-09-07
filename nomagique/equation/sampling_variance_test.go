package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestSamplingVariance(t *testing.T) {
	Convey("Given complete and partial context matches", t, func() {
		for _, fixture := range []struct{ depth, length, support, variance, expected float64 }{
			{3, 3, 4, 16, 4}, {1, 3, 4, 16, 12}, {0, 4, 2, 16, 16}, {0, 0, 0, 16, 16},
		} {
			actual, err := equation.SamplingVariance(fixture.depth, fixture.length, fixture.support, fixture.variance)
			So(err, ShouldBeNil)
			So(actual, ShouldEqual, fixture.expected)
			connected, err := transport.Evaluate[float64](equation.NewSamplingVariance(), core.Record(map[string]any{
				"depth": fixture.depth, "context_length": fixture.length, "support": fixture.support, "variance": fixture.variance,
			}))
			So(err, ShouldBeNil)
			So(connected, ShouldEqual, fixture.expected)
		}
		Convey("An impossible depth is an explicit domain failure", func() {
			_, err := equation.SamplingVariance(4, 3, 4, 16)
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkSamplingVariance(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := equation.SamplingVariance(1, 3, 4, 16); err != nil {
			b.Fatal(err)
		}
	}
}
