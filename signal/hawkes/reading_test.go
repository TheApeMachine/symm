package hawkes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/telemetry"
)

func TestTemperatureScaledGates(t *testing.T) {
	testconfig.Load(t)

	Convey("Given hot macro temperature", t, func() {
		viper.Set("trading.entry.temperature_scale", 0.5)
		telemetry.SharedSurpriseIndex().Record("fluid", 3, 1)

		gates, gatesReady := hawkes.FitGatesFromHistory(
			[]float64{0.7, 0.75, 0.8, 0.82},
			[]float64{0.05, 0.08, 0.1, 0.12},
		)

		So(gatesReady, ShouldBeTrue)

		scaled := temperatureScaledGates(gates)

		Convey("It should raise the frenzy asymmetry bar", func() {
			So(scaled.FrenzyAsymmetry, ShouldBeGreaterThan, gates.FrenzyAsymmetry)
			So(scaled.SaturationRadius, ShouldEqual, gates.SaturationRadius)
		})
	})
}
