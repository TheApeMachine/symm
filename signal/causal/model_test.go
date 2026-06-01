package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAssociationEffectFor(t *testing.T) {
	Convey("Given flow-driven training history", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 8)
		roles := normalRoles()

		association := associationEffectFor(samples, roles)

		Convey("It should read positive association between flow and velocity", func() {
			So(association, ShouldBeGreaterThan, 0)
		})
	})
}

func TestKernelBackdoorEffectFor(t *testing.T) {
	Convey("Given flow-driven training history", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 8)
		roles := normalRoles()

		effect := kernelBackdoorEffectFor(samples, roles)

		Convey("It should estimate a positive backdoor-adjusted intervention", func() {
			So(effect, ShouldBeGreaterThan, 0)
		})
	})
}

func TestBackdoorEffectFor(t *testing.T) {
	Convey("Given flow-driven training history", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 8)
		roles := normalRoles()

		effect := backdoorEffectFor(samples, roles)

		Convey("It should estimate a linear backdoor effect", func() {
			So(effect, ShouldBeGreaterThan, 0)
		})
	})
}

func TestFitStructuralFor(t *testing.T) {
	Convey("Given ladder training history", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 8)
		roles := normalRoles()

		coef, ok := fitStructuralFor(samples, roles)

		Convey("It should fit a linear structural model", func() {
			So(ok, ShouldBeTrue)
			So(len(coef.model.coefficients), ShouldBeGreaterThan, 0)
		})
	})
}

func TestCounterfactualUpliftFor(t *testing.T) {
	Convey("Given a fitted linear structural model", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 8)
		roles := normalRoles()

		coef, ok := fitStructuralFor(samples, roles)

		So(ok, ShouldBeTrue)

		current := samples[len(samples)-1]
		intervention := flowInterventionLevelFor(samples, roles)
		uplift := counterfactualUpliftFor(current, coef, intervention, roles)

		Convey("It should score linear counterfactual uplift", func() {
			So(uplift, ShouldNotEqual, 0)
		})
	})
}

func TestPearsonAndDot(t *testing.T) {
	Convey("Given aligned vectors", t, func() {
		left := []float64{1, 2, 3, 4}
		right := []float64{2, 4, 6, 8}

		Convey("It should report perfect correlation and dot product", func() {
			So(pearson(left, right), ShouldAlmostEqual, 1, 1e-9)
			So(dot(left, right), ShouldEqual, 60)
		})
	})
}

func TestPairConditionNumber(t *testing.T) {
	Convey("Given decoupled liquidity and flow features", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 8)

		condition := pairConditionNumber(samples)

		Convey("It should stay below the panic switch", func() {
			So(condition, ShouldBeGreaterThan, 0)
			So(condition, ShouldBeLessThan, 1000)
		})
	})
}

func BenchmarkAssociationEffectFor(b *testing.B) {
	samples := ladderTrainingSamples(minCausalHistory + 8)
	roles := normalRoles()

	b.ReportAllocs()

	for b.Loop() {
		_ = associationEffectFor(samples, roles)
	}
}

func BenchmarkKernelBackdoorEffectFor(b *testing.B) {
	samples := ladderTrainingSamples(minCausalHistory + 8)
	roles := normalRoles()

	b.ReportAllocs()

	for b.Loop() {
		_ = kernelBackdoorEffectFor(samples, roles)
	}
}
