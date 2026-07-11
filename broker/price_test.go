package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestPriceTickerAck(t *testing.T) {
	Convey("Given a ticker envelope", t, func() {
		price := &Price{
			status:  types.READY,
			fees:    &sync.Map{},
			tickers: &sync.Map{},
			ui:      make(chan []byte, 1),
		}

		price.TickerAck([]byte(`{
			"channel":"ticker",
			"type":"update",
			"data":[{"symbol":"BTC/USD","last":"42000.0","bid":"41999.0","ask":"42001.0"}]
		}`))

		Convey("It should cache ticker rows as pointers", func() {
			ticker, err := price.Get("BTC/USD")

			So(err, ShouldBeNil)
			So(ticker, ShouldNotBeNil)
			So(ticker.Symbol, ShouldEqual, "BTC/USD")
			So(ticker.Last.Float64(), ShouldEqual, 42000.0)
		})

		Convey("It should publish without panicking", func() {
			So(func() { price.Publish() }, ShouldNotPanic)
		})
	})
}

func TestPriceWithFriction(t *testing.T) {
	Convey("Given a price stream with a known taker fee", t, func() {
		price := &Price{
			status:  types.READY,
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
		price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.0026"})
		last, err := decimal.NewFromString("1")

		So(err, ShouldBeNil)

		price.tickers.Store("BTC/USD", &kraken.TickerData{
			Symbol: "BTC/USD",
			Last:   last,
		})

		Convey("When WithFriction is requested for unit quantity", func() {
			net, err := price.WithFriction("BTC/USD", 1)

			Convey("Then it returns the all-in round-trip taker quote", func() {
				So(err, ShouldBeNil)
				So(net.Float64(), ShouldAlmostEqual, 1.00520576, 1e-8)
			})
		})
	})
}

func BenchmarkPriceWithFriction(b *testing.B) {
	price := &Price{
		status:  types.READY,
		fees:    &sync.Map{},
		tickers: &sync.Map{},
	}
	price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.0026"})
	last, err := decimal.NewFromString("1")

	if err != nil {
		b.Fatal(err)
	}

	price.tickers.Store("BTC/USD", &kraken.TickerData{
		Symbol: "BTC/USD",
		Last:   last,
	})

	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.WithFriction("BTC/USD", 1)
	}
}
