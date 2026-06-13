package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestSurpriseRegistryThreshold(t *testing.T) {
	Convey("Given a surprise registry under a hot market", t, func() {
		registry := NewSurpriseRegistry()
		registry.SetTemperature(0.8)

		for index := 0; index < 40; index++ {
			registry.Observe(logic.SourceFluid, 1.1+float64(index%3)*0.05)
		}

		threshold := registry.Threshold(logic.SourceFluid)

		Convey("It should derive a threshold above the cold-market floor", func() {
			So(threshold, ShouldBeGreaterThan, warmupSurpriseThreshold(0))
		})
	})
}
