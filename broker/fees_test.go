package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestMakerFeePctFromViper(t *testing.T) {
	Convey("Given configured maker fee percent", t, func() {
		viper.Set("trading.paper.maker_fee_pct", 0.30)
		defer viper.Set("trading.paper.maker_fee_pct", 0.0)

		So(MakerFeePctFromViper(), ShouldAlmostEqual, 0.30, 1e-9)
	})

	Convey("Given no configured maker fee", t, func() {
		viper.Set("trading.paper.maker_fee_pct", 0.0)

		So(MakerFeePctFromViper(), ShouldAlmostEqual, defaultMakerFeePctPercent, 1e-9)
	})
}

func TestTakerFeePctFromViper(t *testing.T) {
	Convey("Given configured taker fee percent", t, func() {
		viper.Set("trading.paper.taker_fee_pct", 0.35)
		defer viper.Set("trading.paper.taker_fee_pct", 0.0)

		So(TakerFeePctFromViper(), ShouldAlmostEqual, 0.35, 1e-9)
	})
}
