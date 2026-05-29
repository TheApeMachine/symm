package trader

import (
	"testing"

	"github.com/theapemachine/symm/config"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfiguredExecutionFallbackTicks(t *testing.T) {
	Convey("Given explicit fallback ticks", t, func() {
		original := *config.System
		config.System.ExecutionMakerFallbackTicks = 7
		t.Cleanup(func() { *config.System = original })

		Convey("It should use the configured value", func() {
			So(configuredExecutionFallbackTicks(), ShouldEqual, 7)
		})
	})
}
