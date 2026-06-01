package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFitNonLinearStructuralFor(t *testing.T) {
	Convey("Given nonlinear flow-to-velocity history", t, func() {
		samples := upliftTrainingSamples(minCausalHistory + 12)
		roles := normalRoles()

		model, ok := fitNonLinearStructuralFor(samples, roles)

		Convey("It should fit at least one stump", func() {
			So(ok, ShouldBeTrue)
			So(len(model.stumps), ShouldBeGreaterThan, 0)
		})
	})
}

func TestNonLinearCounterfactualUpliftFor(t *testing.T) {
	Convey("Given a fitted nonlinear structural model", t, func() {
		samples := upliftTrainingSamples(minCausalHistory + 12)
		roles := normalRoles()

		model, ok := fitNonLinearStructuralFor(samples, roles)

		So(ok, ShouldBeTrue)

		current := samples[len(samples)-1]
		intervention := flowInterventionLevelFor(samples, roles)
		uplift := nonLinearCounterfactualUpliftFor(current, model, intervention, roles)

		Convey("It should score counterfactual uplift", func() {
			So(uplift, ShouldNotEqual, 0)
		})
	})
}

func TestNonLinearModelPredict(t *testing.T) {
	Convey("Given a fitted stump ensemble", t, func() {
		samples := upliftTrainingSamples(minCausalHistory + 12)
		model, ok := fitNonLinearStructuralFor(samples, normalRoles())

		So(ok, ShouldBeTrue)

		prediction, err := model.Predict(samples[len(samples)-1].nodes[:], -1, 0)

		Convey("It should emit a finite prediction", func() {
			So(err, ShouldBeNil)
			So(prediction, ShouldNotEqual, 0)
		})
	})
}

func BenchmarkFitNonLinearStructuralFor(b *testing.B) {
	samples := upliftTrainingSamples(minCausalHistory + 12)
	roles := normalRoles()

	b.ReportAllocs()

	for b.Loop() {
		_, _ = fitNonLinearStructuralFor(samples, roles)
	}
}
