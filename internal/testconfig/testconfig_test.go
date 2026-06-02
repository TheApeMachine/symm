package testconfig

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestLoad(t *testing.T) {
	Convey("Given the repo config", t, func() {
		Load(t)

		Convey("It should expose trading limits from config.yml", func() {
			So(viper.GetFloat64("trading.max_spread_bps"), ShouldBeGreaterThan, 0)
			So(viper.GetString("market.quote_currency"), ShouldNotBeBlank)
		})
	})
}

func BenchmarkLoad(b *testing.B) {
	for b.Loop() {
		viper.Reset()
		MustLoad()
	}
}
