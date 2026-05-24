package pumpdump

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestWarmFromOHLC(t *testing.T) {
	convey.Convey("Given historical candles", t, func() {
		trackStore := &TrackStore{
			bySymbol: make(map[string]*SymbolTrack),
		}

		candles := map[string][]engine.OHLCCandle{
			"BTC/EUR": {
				{Open: 100, High: 110, Low: 95, Close: 105, Volume: 12},
				{Open: 105, High: 112, Low: 104, Close: 110, Volume: 15},
				{Open: 110, High: 111, Low: 109, Close: 110.5, Volume: 8},
			},
		}

		convey.Convey("It should seed volume and price-move history", func() {
			trackStore.WarmFromOHLC(candles)

			track := trackStore.bySymbol["BTC/EUR"]
			convey.So(track, convey.ShouldNotBeNil)
			convey.So(len(track.volumes), convey.ShouldEqual, 2)
			convey.So(len(track.priceMoves), convey.ShouldEqual, 2)
			convey.So(track.lastPrice, convey.ShouldEqual, 110)
		})
	})
}
