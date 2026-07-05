package causal

import (
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/logic"
)

func TestEngineMeasureCounterfactualReady(testingTB *testing.T) {
	Convey("Given a causal engine output without rung-three values", testingTB, func() {
		engine := NewEngine()
		output := engineOutput()

		measurement, err := engine.measure("BTC/USD", time.Now().UTC(), output, true, nil)

		Convey("It should not mark counterfactual readiness", func() {
			So(err, ShouldBeNil)
			So(measurement, ShouldNotBeNil)
			So(measurement.CounterfactualReady, ShouldBeFalse)
		})
	})

	Convey("Given a causal engine output with intervention and uplift", testingTB, func() {
		engine := NewEngine()
		output := engineOutput()
		output.InterventionScore = 1
		output.UpliftScore = 1

		measurement, err := engine.measure("BTC/USD", time.Now().UTC(), output, true, nil)

		Convey("It should mark counterfactual readiness", func() {
			So(err, ShouldBeNil)
			So(measurement, ShouldNotBeNil)
			So(measurement.CounterfactualReady, ShouldBeTrue)
		})
	})
}

func engineOutput() algorithm.PearlOutput {
	category := logic.CategoryEndogenousAlpha
	baseline := 1 / float64(len(causalCategories))

	return algorithm.PearlOutput{
		Value:         float64(logic.CategoryIndex(category)),
		Confidence:    1,
		EntryBaseline: baseline,
		ExitBaseline:  baseline,
		Strength:      1,
		AlphaScore:    1,
		Distribution: map[string]float64{
			strconv.Itoa(logic.CategoryIndex(category)): 1,
		},
	}
}
