package market

import (
	"errors"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestInstrumentAssetPair(t *testing.T) {
	convey.Convey("Given a websocket instrument record", t, func() {
		instrument := Instrument{
			Symbol:  "BTC/EUR",
			Base:    "BTC",
			Quote:   "EUR",
			Status:  PairStatusOnline,
			CostMin: 0.45,
		}

		convey.Convey("It should map into an asset pair", func() {
			pair := instrument.AssetPair()
			convey.So(pair.Wsname, convey.ShouldEqual, "BTC/EUR")
			convey.So(pair.Quote, convey.ShouldEqual, "EUR")
			convey.So(pair.Costmin, convey.ShouldEqual, "0.45")
		})
	})
}

func TestInstrumentMessageParse(t *testing.T) {
	convey.Convey("Given a Kraken v2 instrument websocket frame", t, func() {
		convey.Convey("It should extract snapshot pairs", func() {
			var instrumentMessage InstrumentMessage

			err := instrumentMessage.Parse(sampleInstrumentFrame)
			convey.So(err, convey.ShouldBeNil)
			convey.So(instrumentMessage.Type, convey.ShouldEqual, InstrumentUpdateTypeSnapshot)
			convey.So(len(instrumentMessage.Data.Pairs), convey.ShouldEqual, 2)
			convey.So(instrumentMessage.Data.Pairs[0].Symbol, convey.ShouldEqual, "BTC/EUR")
		})
	})
}

func TestInstrumentMessageParseRejectsNonInstrumentChannel(t *testing.T) {
	convey.Convey("Given a non-instrument websocket frame", t, func() {
		convey.Convey("It should reject the payload", func() {
			var instrumentMessage InstrumentMessage

			err := instrumentMessage.Parse(sampleTradeFrame)
			convey.So(errors.Is(err, ErrNotInstrument), convey.ShouldBeTrue)
		})
	})
}
