package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestSelectRoles(t *testing.T) {
	Convey("Given a normal training history", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 4)

		roles, inverted := selectRoles(samples, 0)

		Convey("It should keep flow as treatment", func() {
			So(inverted, ShouldBeFalse)
			So(roles.treatment, ShouldEqual, localFlowNode)
			So(roles.label, ShouldEqual, regimeNormal)
		})
	})

	Convey("Given contagion above the configured break", t, func() {
		samples := ladderTrainingSamples(minCausalHistory + 4)

		roles, inverted := selectRoles(samples, viper.GetViper().GetFloat64("signals.causal.contagion_break")+0.05)

		Convey("It should invert to liquidity treatment", func() {
			So(inverted, ShouldBeTrue)
			So(roles.treatment, ShouldEqual, liquidityNode)
			So(roles.label, ShouldEqual, regimePanic)
		})
	})
}

func BenchmarkSelectRoles(b *testing.B) {
	samples := ladderTrainingSamples(minCausalHistory + 8)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = selectRoles(samples, 0.2)
	}
}
