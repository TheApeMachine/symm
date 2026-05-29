package market

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestTickerFrame(t *testing.T) {
	convey.Convey("Given a ticker websocket frame", t, func() {
		message := &public.SocketMessage{}

		convey.Convey("It should decode ticker rows", func() {
			convey.So(sonic.Unmarshal(sampleTickerFrame, message), convey.ShouldBeNil)

			var rows []TickerUpdate

			convey.So(sonic.Unmarshal(message.Data, &rows), convey.ShouldBeNil)
			convey.So(len(rows), convey.ShouldEqual, 1)
			convey.So(rows[0].Symbol, convey.ShouldEqual, "BTC/EUR")
			convey.So(rows[0].Last, convey.ShouldEqual, 50000.0)
		})
	})
}

func TestBookFrame(t *testing.T) {
	convey.Convey("Given a book websocket frame", t, func() {
		message := &public.SocketMessage{}

		convey.Convey("It should decode book rows", func() {
			convey.So(sonic.Unmarshal(sampleBookFrame, message), convey.ShouldBeNil)

			var rows []BookUpdate

			convey.So(sonic.Unmarshal(message.Data, &rows), convey.ShouldBeNil)
			convey.So(len(rows), convey.ShouldEqual, 1)
			convey.So(rows[0].Symbol, convey.ShouldEqual, "BTC/EUR")
			convey.So(len(rows[0].Bids), convey.ShouldEqual, 1)
			convey.So(rows[0].Bids[0].Price, convey.ShouldEqual, 49999.5)
		})
	})
}

func TestTradeFrame(t *testing.T) {
	convey.Convey("Given a trade websocket frame", t, func() {
		message := &public.SocketMessage{}

		convey.Convey("It should decode trade rows", func() {
			convey.So(sonic.Unmarshal(sampleTradeFrame, message), convey.ShouldBeNil)

			var rows []TradeUpdate

			convey.So(sonic.Unmarshal(message.Data, &rows), convey.ShouldBeNil)
			convey.So(len(rows), convey.ShouldEqual, 2)
			convey.So(rows[0].Side, convey.ShouldEqual, "buy")
		})
	})
}

func TestCandleFrame(t *testing.T) {
	convey.Convey("Given an ohlc websocket frame", t, func() {
		message := &public.SocketMessage{}

		convey.Convey("It should decode candle rows", func() {
			convey.So(sonic.Unmarshal(sampleCandleFrame, message), convey.ShouldBeNil)

			var rows []CandleUpdate

			convey.So(sonic.Unmarshal(message.Data, &rows), convey.ShouldBeNil)
			convey.So(len(rows), convey.ShouldEqual, 1)
			convey.So(rows[0].Close, convey.ShouldEqual, 50000.0)
		})
	})
}

func TestInstrumentFrame(t *testing.T) {
	convey.Convey("Given an instrument websocket frame", t, func() {
		message := &public.SocketMessage{}

		convey.Convey("It should decode instrument data", func() {
			convey.So(sonic.Unmarshal(sampleInstrumentFrame, message), convey.ShouldBeNil)

			var update InstrumentUpdate

			convey.So(sonic.Unmarshal(message.Data, &update), convey.ShouldBeNil)
			convey.So(len(update.Pairs), convey.ShouldEqual, 2)
			convey.So(update.Pairs[0].Symbol, convey.ShouldEqual, "BTC/EUR")
		})
	})
}

func TestLevel3Frame(t *testing.T) {
	convey.Convey("Given a level3 websocket frame", t, func() {
		message := &public.SocketMessage{}

		convey.Convey("It should decode level3 rows", func() {
			convey.So(sonic.Unmarshal(sampleLevel3Frame, message), convey.ShouldBeNil)

			var rows []Level3Update

			convey.So(sonic.Unmarshal(message.Data, &rows), convey.ShouldBeNil)
			convey.So(len(rows), convey.ShouldEqual, 1)
			convey.So(rows[0].Bids[0].OrderID, convey.ShouldEqual, "OABC-123")
		})
	})
}
