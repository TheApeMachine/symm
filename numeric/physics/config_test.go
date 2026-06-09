package physics

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestNewConfigFromViper(t *testing.T) {
	convey.Convey("Given manifold signal config", t, func() {
		viper.Set("signals.manifold.grid_x", 16)
		viper.Set("signals.manifold.grid_y", 1)
		viper.Set("signals.manifold.grid_z", 8)
		viper.Set("signals.manifold.tick_size", 0.01)
		viper.Set("signals.manifold.grid_half_width", 16)
		viper.Set("signals.manifold.integration_interval", "100ms")
		viper.Set("signals.manifold.max_modes", 8)

		config, err := NewConfigFromViper()

		convey.Convey("It should derive a positive cell volume and quantum scales", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(config.GridX, convey.ShouldEqual, uint32(16))
			convey.So(config.CellVolume(), convey.ShouldBeGreaterThan, 0)
			convey.So(config.HbarEffective(), convey.ShouldBeGreaterThan, 0)
			convey.So(config.GInteraction(), convey.ShouldBeGreaterThan, 0)
		})
	})
}
