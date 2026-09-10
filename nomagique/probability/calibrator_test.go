package probability_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestCalibratorRetention(t *testing.T) {
	Convey("Rank is computed against the prior window, then the sample is retained", t, func() {
		checkCalibrator(probability.NewCalibrator(collection.NewTail[float64](4)), 4)
		checkCalibrator(probability.NewCalibrator(nil), 0)
	})

	Convey("Non-finite samples are refused without changing the prior window", t, func() {
		for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			node := probability.NewCalibrator(nil)
			_, err := transport.Evaluate(node, transport.Values(10.0))
			So(err, ShouldBeNil)

			out := tests.CollectSeq(node.Next(transport.Values(bad)))
			So(len(out), ShouldEqual, 0)
			So(node.Error(), ShouldNotBeNil)

			got := tests.CollectSeq(node.Next(transport.Values(5.0)))
			So(len(got), ShouldEqual, 1)
			So(got[0].PriorCount, ShouldEqual, 1)
			So(got[0].Value, ShouldEqual, 1)
		}
	})
}

func checkCalibrator(node *probability.Calibrator, capacity int) {
	history := []float64{}

	for _, sample := range []float64{10, 20, 30, 15, 40, 50, 1, 5, 99, 4} {
		want := 0.0

		for _, prior := range history {
			if prior > sample {
				want++
			}
		}

		if len(history) > 0 {
			want /= float64(len(history))
		}

		got, err := transport.Evaluate(node, transport.Values(sample))
		So(err, ShouldBeNil)
		So(got.Value, ShouldEqual, want)
		So(got.PriorCount, ShouldEqual, float64(len(history)))
		So(got.Ready, ShouldEqual, len(history) > 0)
		history = append(history, sample)

		if capacity > 0 && len(history) > capacity {
			history = history[len(history)-capacity:]
		}
	}
}

func BenchmarkCalibrator(b *testing.B) {
	node := probability.NewCalibrator(collection.NewTail[float64](4))
	b.ReportAllocs()

	for b.Loop() {
		out := tests.CollectSeq(node.Next(transport.Values(1.234)))

		if len(out) != 1 {
			b.Fatal("expected one calibrator record")
		}
	}

	if err := node.Error(); err != nil {
		b.Fatal(err)
	}
}

func TestCalibratorNext(t *testing.T) {
	Convey("A tail of three is the prior for the next rank", t, func() {
		calibrator := probability.NewCalibrator(collection.NewTail[float64](3))
		history := []float64{}

		for _, sample := range []float64{4, 2, 3, 1, 5} {
			out, err := transport.Evaluate(calibrator, transport.Values(sample))
			So(err, ShouldBeNil)
			want := 0.0

			for _, prior := range history {
				if prior > sample {
					want++
				}
			}

			if len(history) > 0 {
				want /= float64(len(history))
			}

			So(out.Value, ShouldEqual, want)
			So(out.PriorCount, ShouldEqual, float64(len(history)))
			So(out.Ready, ShouldEqual, len(history) > 0)
			history = append(history, sample)

			if len(history) > 3 {
				history = history[1:]
			}
		}
	})
}
