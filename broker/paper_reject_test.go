package broker

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestShouldRejectPaperOrderAtCertainRate(t *testing.T) {
	convey.Convey("Given a 100% paper reject rate", t, func() {
		original := config.System.PaperOrderRejectRate
		config.System.PaperOrderRejectRate = 1
		t.Cleanup(func() { config.System.PaperOrderRejectRate = original })

		convey.Convey("It should always reject", func() {
			convey.So(ShouldRejectPaperOrder(), convey.ShouldNotBeNil)
		})
	})
}
