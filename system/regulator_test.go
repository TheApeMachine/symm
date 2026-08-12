package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewRegulatorConfig(t *testing.T) {
	Convey("Given the configured online optimization policy", t, func() {
		config := NewRegulatorConfig()

		Convey("It should expose a bounded history and posterior confidence", func() {
			So(config.HistoryCapacity, ShouldBeGreaterThan, 0)
			So(config.OptimizationConfidence, ShouldBeGreaterThan, 0.5)
			So(config.OptimizationConfidence, ShouldBeLessThan, 1.0)
		})
	})
}
