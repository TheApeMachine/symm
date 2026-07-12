package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/types"
)

func TestTradeOn(t *testing.T) {
	Convey("Given a lossless trade ring at capacity", t, func() {
		capacity := viper.GetInt("signals.feed_ring_capacity")
		defer viper.Set("signals.feed_ring_capacity", capacity)
		viper.Set("signals.feed_ring_capacity", 2)
		trade := NewTrade(&Signal{}, nil)

		trade.On([]byte("first"))
		trade.On([]byte("second"))
		trade.On([]byte("rejected"))

		Convey("It should count the rejection without racing feed status", func() {
			So(trade.ring.Rejected(), ShouldEqual, uint64(1))
			So(trade.Status(), ShouldEqual, types.INITIALIZING)
		})
	})
}
