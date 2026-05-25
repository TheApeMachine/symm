package numeric

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/numeric/adaptive"
)

func TestAccumulateNext(t *testing.T) {
	t.Parallel()

	Convey("Given accumulate stages in a pipeline", t, func() {
		press := NewDerived(WithDynamics(adaptive.NewProduct(), adaptive.NewEMA(0)))
		spread := NewDerived(WithDynamics(adaptive.NewEMA(0), adaptive.NewCompression(0)))

		for range 8 {
			_, _ = spread.Push(20)
		}

		scored := NewScored(
			mustClassifier(),
			NewAccumulate(press, func(values []float64) []float64 {
				return values[1:3]
			}),
			NewAccumulate(spread, func(values []float64) []float64 {
				return values[3:4]
			}),
			NewScaleIndex(0),
		)

		confidence, err := scored.Push(2, 0.8, 0.8, 10, 1, 1)

		So(err, ShouldBeNil)
		So(confidence, ShouldBeGreaterThan, 0)
	})
}

func TestDerivedPushAsDynamic(t *testing.T) {
	t.Parallel()

	Convey("Given a nested Derived stage", t, func() {
		chain := NewDerived(WithDynamics(
			NewDerived(WithDynamics(adaptive.NewProduct(), adaptive.NewEMA(0))),
		))

		out, err := chain.Push(0.8, 0.3)

		So(err, ShouldBeNil)
		So(out, ShouldAlmostEqual, 0.24, 1e-9)
	})
}

func mustClassifier() *adaptive.Classifier {
	classifier, _ := adaptive.NewClassifier(
		[]float64{-0.001, 0.001},
		[]float64{0, 1, 2},
		[]string{"dump", "precursor", "actual_pump"},
	)

	return classifier
}
