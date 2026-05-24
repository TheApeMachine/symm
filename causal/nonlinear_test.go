package causal

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestFitNonLinearStructural(t *testing.T) {
	samples := make([]causalSample, 0, minCausalHistory)

	for index := 0; index < minCausalHistory; index++ {
		flow := float64(index) / float64(minCausalHistory)
		samples = append(samples, causalSample{
			macroMomentum: 0.1,
			liquidity:     1,
			localFlow:     flow,
			priceVelocity: flow * flow,
		})
	}

	convey.Convey("Given non-linear flow-velocity samples", t, func() {
		model, ok := fitNonLinearStructural(samples)

		convey.Convey("It should fit a stump ensemble", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(len(model.stumps), convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestKernelBackdoorFlowEffect(t *testing.T) {
	samples := make([]causalSample, 0, minCausalHistory)

	for index := 0; index < minCausalHistory; index++ {
		flow := float64(index+1) / 10
		samples = append(samples, causalSample{
			macroMomentum: 0.05,
			liquidity:     1,
			localFlow:     flow,
			priceVelocity: flow * 1.5,
		})
	}

	effect := kernelBackdoorFlowEffect(samples)

	if effect <= 0 {
		t.Fatalf("expected positive kernel backdoor effect, got %v", effect)
	}
}
