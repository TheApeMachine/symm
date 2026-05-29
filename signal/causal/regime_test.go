package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/config"
)

func wellConditionedSamples() []causalSample {
	samples := make([]causalSample, 0, minCausalHistory+4)

	for index := 0; index < minCausalHistory+4; index++ {
		samples = append(samples, newCausalSample(
			float64(index%4)*0.004,  // macro: small, periodic
			1+float64(index%5)*0.13, // liquidity: periodic, independent of flow
			float64(index)*0.4,      // flow: monotone ramp
			float64(index%3)*0.05,   // velocity
		))
	}

	return samples
}

func collinearSamples() []causalSample {
	samples := make([]causalSample, 0, minCausalHistory+4)

	for index := 0; index < minCausalHistory+4; index++ {
		flow := float64(index)*0.4 + 0.1

		samples = append(samples, newCausalSample(
			float64(index%4)*0.004,
			2*flow, // liquidity is an exact affine image of flow: edges collapse
			flow,
			float64(index%3)*0.05,
		))
	}

	return samples
}

func TestSelectRolesHoldsNormalRegimeWhenStable(t *testing.T) {
	Convey("Given well-conditioned samples and calm cross-asset correlation", t, func() {
		roles, inverted := selectRoles(wellConditionedSamples(), 0)

		Convey("It should keep the normal regime: flow is the treatment", func() {
			So(inverted, ShouldBeFalse)
			So(roles.label, ShouldEqual, regimeNormal)
			So(roles.treatment, ShouldEqual, localFlowNode)
			So(roles.controls, ShouldResemble, []int{macroMomentumNode, liquidityNode})
		})
	})
}

func TestSelectRolesInvertsOnLiquidityFlowCollinearity(t *testing.T) {
	Convey("Given liquidity and flow collapsed onto one axis", t, func() {
		roles, inverted := selectRoles(collinearSamples(), 0)

		Convey("It should flip to the panic regime: liquidity is the treatment", func() {
			So(inverted, ShouldBeTrue)
			So(roles.label, ShouldEqual, regimePanic)
			So(roles.treatment, ShouldEqual, liquidityNode)
			So(roles.controls, ShouldResemble, []int{macroMomentumNode})
		})
	})
}

func TestSelectRolesInvertsOnContagionBreak(t *testing.T) {
	Convey("Given well-conditioned samples but a cross-asset contagion spike", t, func() {
		contagion := config.System.CausalContagionBreak + 0.05
		roles, inverted := selectRoles(wellConditionedSamples(), contagion)

		Convey("It should flip to the panic regime on contagion alone", func() {
			So(inverted, ShouldBeTrue)
			So(roles.label, ShouldEqual, regimePanic)
		})
	})
}
