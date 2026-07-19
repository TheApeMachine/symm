package fluid

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSignalOnTicker(t *testing.T) {
	Convey("Given a Fluid signal wired to the ticker channel", t, func() {
		signal := &Signal{registry: NewSyncRegistry(), ticker: NewTicker(NewSyncRegistry()), tickerCache: tickerCache()}
		payload := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","bid":100,"ask":101,"last":100.5,"volume":10,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When a ticker frame arrives", func() {
			signal.observeTicker(payload)

			Convey("Then the row should accumulate in the ticker cache", func() {
				So(len(tickerRows(signal.tickerCache)), ShouldEqual, 1)
				So(tickerRows(signal.tickerCache)[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})

		Convey("When an empty frame arrives", func() {
			signal.observeTicker(nil)

			Convey("Then nothing should be cached", func() {
				So(tickerRows(signal.tickerCache), ShouldBeEmpty)
			})
		})
	})
}

func TestSignalOnTrade(t *testing.T) {
	Convey("Given a Fluid signal wired to the trade channel", t, func() {
		signal := &Signal{tradeCache: tradeCache()}
		payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100.5,"qty":1.25,"ord_type":"market","trade_id":1,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When a trade frame arrives", func() {
			signal.observeTrade(payload)

			Convey("Then the row should accumulate in the trade cache", func() {
				So(len(tradeRows(signal.tradeCache)), ShouldEqual, 1)
				So(tradeRows(signal.tradeCache)[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func TestSignalOnBook(t *testing.T) {
	Convey("Given a Fluid signal without instrument metadata", t, func() {
		signal := &Signal{bookCache: bookCache()}
		payload := []byte(`{"channel":"book","type":"update","data":[{"symbol":"BTC/USD","bids":[{"price":100,"qty":10}],"asks":[{"price":101,"qty":10}],"checksum":1,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When a book frame arrives", func() {
			signal.observeBook(payload)

			Convey("Then the row should accumulate with a zero price increment", func() {
				So(len(bookRows(signal.bookCache)), ShouldEqual, 1)
				So(bookRows(signal.bookCache)[0].Symbol, ShouldEqual, "BTC/USD")
				So(bookRows(signal.bookCache)[0].PriceIncrement.Sign(), ShouldEqual, 0)
			})
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given a Fluid signal with ticker, trade, and book rows cached", t, func() {
		registry := NewSyncRegistry()
		signal := &Signal{
			registry:    registry,
			ticker:      NewTicker(registry),
			trade:       NewTrade(registry),
			book:        NewBook(registry),
			tickerCache: tickerCache(),
			tradeCache:  tradeCache(),
			bookCache:   bookCache(),
		}
		fixture := &symbolBookFixture{symbol: "BTC/USD"}
		row := fixture.snapshot(100, 5, 101, 5)
		row.Timestamp = time.Date(2026, 7, 10, 4, 0, 0, 0, time.UTC)
		signal.bookCache = bookCache(row)

		Convey("When measuring a tick with no market state yet", func() {
			frame := &types.MarketFrame{CrossSection: types.NewCrossSection()}
			measurements, err := signal.Calculate(frame)

			Convey("Then it should measure an empty frame without erroring", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeEmpty)
			})
		})
	})
}
