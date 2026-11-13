package cmd

import (
	"testing"

	"github.com/spf13/viper"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadEmbeddedConfig(t *testing.T) {
	Convey("Setup", t, func() {
		viper.Reset()

		Convey("When loadEmbeddedConfig reads the baked config", func() {
			err := loadEmbeddedConfig()

			Convey("Viper holds the embedded learning threshold_start", func() {
				So(err, ShouldBeNil)
				So(viper.GetFloat64("learning.threshold_start"), ShouldEqual, 0.5)
				So(viper.GetString("market.model"), ShouldEqual, "real")
			})
		})
	})
}
