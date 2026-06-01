package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestDAGLinearModel(t *testing.T) {
	Convey("Given a causal node table", t, func() {
		table := tableFromSamples(ladderTrainingSamples(minCausalHistory + 8))
		roles := normalRoles()

		model, err := table.LinearModel(roles.predictors()...)

		Convey("It should fit structural coefficients", func() {
			So(err, ShouldBeNil)
			So(len(model.coefficients), ShouldEqual, len(roles.predictors())+1)
		})
	})
}

func TestDAGBackdoorEffect(t *testing.T) {
	Convey("Given flow treatment with macro and liquidity controls", t, func() {
		table := tableFromSamples(ladderTrainingSamples(minCausalHistory + 8))
		roles := normalRoles()

		effect, err := table.BackdoorEffect(roles.treatment, roles.controls...)

		Convey("It should estimate a residualized backdoor effect", func() {
			So(err, ShouldBeNil)
			So(effect, ShouldBeGreaterThan, 0)
		})
	})
}

func TestDAGCounterfactualUplift(t *testing.T) {
	Convey("Given a fitted linear structural model", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 8)
		table := tableFromSamples(samples)
		roles := normalRoles()

		model, err := table.LinearModel(roles.predictors()...)

		So(err, ShouldBeNil)

		current := samples[len(samples)-1]
		intervention := current.value(localFlowNode) * 1.5

		uplift, err := model.CounterfactualUplift(current.nodes[:], roles.treatment, intervention)

		Convey("It should score do(treatment) uplift over observed", func() {
			So(err, ShouldBeNil)
			So(uplift, ShouldNotEqual, 0)
		})
	})
}

func TestDAGPairConditionNumber(t *testing.T) {
	Convey("Given collinear liquidity and flow columns", t, func() {
		table := tableFromSamples(collinearTrainingSamples(minCausalHistory + 8))

		condition, err := table.PairConditionNumber(liquidityNode, localFlowNode)

		Convey("It should diverge as the axes collapse", func() {
			So(err, ShouldBeNil)
			So(condition, ShouldBeGreaterThan, viper.GetViper().GetFloat64("signals.causal.condition_switch"))
		})
	})
}

func TestDAGPercentile(t *testing.T) {
	Convey("Given a node column", t, func() {
		table := tableFromSamples(ladderTrainingSamples(minCausalHistory + 8))

		value, err := table.Percentile(localFlowNode, 0.75)

		Convey("It should return the requested quantile", func() {
			So(err, ShouldBeNil)
			So(value, ShouldBeGreaterThan, 0)
		})
	})
}

func TestDAGValidationErrors(t *testing.T) {
	Convey("Given an invalid node index", t, func() {
		table := tableFromSamples(ladderTrainingSamples(minCausalHistory + 8))

		_, err := table.column(-1)

		Convey("It should reject out-of-range nodes", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkDAGBackdoorEffect(b *testing.B) {
	table := tableFromSamples(ladderTrainingSamples(minCausalHistory + 8))
	roles := normalRoles()

	b.ReportAllocs()

	for b.Loop() {
		_, _ = table.BackdoorEffect(roles.treatment, roles.controls...)
	}
}
