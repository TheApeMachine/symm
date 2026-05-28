package market

import (
	"testing"

	"github.com/theapemachine/symm/kraken/core"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChannelIsTrade(t *testing.T) {
	Convey("Given Kraken trade channel aliases", t, func() {
		Convey("It should accept both websocket names", func() {
			So(Channel(core.ChannelTrades).IsTrade(), ShouldBeTrue)
			So(Channel("trade").IsTrade(), ShouldBeTrue)
			So(Channel(core.ChannelTicker).IsTrade(), ShouldBeFalse)
		})
	})
}

func TestChannelIsBook(t *testing.T) {
	Convey("Given the book channel", t, func() {
		Convey("It should match only book payloads", func() {
			So(Channel(core.ChannelBook).IsBook(), ShouldBeTrue)
			So(Channel("trade").IsBook(), ShouldBeFalse)
		})
	})
}
