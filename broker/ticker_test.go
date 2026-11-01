package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPriceTicker(t *testing.T) {
	Convey("Price stores ticker quote volume and change", t, func() {
		price := &Price{}
		price.SetTicker("STRK/USD", 120_000, 22.5)

		So(price.VolumeUSD("STRK/USD"), ShouldEqual, 120_000)
		So(price.ChangePct("STRK/USD"), ShouldEqual, 22.5)
		So(price.VolumeUSD("QUIET/USD"), ShouldEqual, 0)
	})
}
