package settings

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestL3Enabled(t *testing.T) {
	Convey("Given market.l3_enabled", t, func() {
		viper.Set("market.l3_enabled", true)
		defer viper.Set("market.l3_enabled", false)

		So(L3Enabled(), ShouldBeTrue)
		So(L3Depth(), ShouldEqual, defaultL3Depth)
	})
}
