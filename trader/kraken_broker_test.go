package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestNewKrakenBrokerLive(t *testing.T) {
	convey.Convey("Given a Kraken broker", t, func() {
		broker := NewKrakenBroker(nil, nil, 0.26)

		convey.Convey("It should report live execution", func() {
			convey.So(broker.Live(), convey.ShouldBeTrue)
		})
	})
}

func TestRoundBaseQty(t *testing.T) {
	convey.Convey("Given lot decimals", t, func() {
		convey.Convey("It should floor to the pair precision", func() {
			convey.So(roundBaseQty(0.123456789, 4), convey.ShouldEqual, 0.1234)
		})
	})
}
