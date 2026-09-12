package probability_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/collection"
	"github.com/theapemachine/symm/nomagique/core"
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
			_Eval := transport.NewEvaluate(node)

			for range _Eval.Next(transport.NewValues(10.0).Next(nil)) {
			}

			err := _Eval.Error()
			So(err, ShouldBeNil)

			out := tests.CollectSeq[probability.CalibratorReading](node.Next(transport.NewValues(bad).Next(nil)))
			So(len(out), ShouldEqual, 0)
			So(node.Error(), ShouldNotBeNil)

			got := tests.CollectSeq[probability.CalibratorReading](node.Next(transport.NewValues(5.0).Next(nil)))
			So(len(got), ShouldEqual, 1)
			So(got[0].PriorCount, ShouldEqual, 1)
			So(got[0].Value, ShouldEqual, 1)
		}
	})
}

func checkCalibrator(node core.Primitive, capacity int) {
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

		gotEval := transport.NewEvaluate(node)
		var got probability.CalibratorReading

		for out := range gotEval.Next(transport.NewValues(sample).Next(nil)) {
			got = *(*probability.CalibratorReading)(out)
		}

		err := gotEval.Error()
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
