package broker

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mock"
)

type entryEconomicsBookConn struct {
	*mock.Conn
	managed *book.Book
}

func (conn *entryEconomicsBookConn) Book(_ string, read func(*book.Book)) {
	read(conn.managed)
}

func TestPriceEntryCost(t *testing.T) {
	Convey("Given current quotes and a valid taker fee", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 5)
		cost, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(1))

		Convey("It should report only present execution and fee facts", func() {
			So(err, ShouldBeNil)
			So(cost.EntryPrice.Float64(), ShouldAlmostEqual, 101, 1e-12)
			So(cost.Midpoint.Float64(), ShouldAlmostEqual, 100.5, 1e-12)
			So(cost.Spread.Float64(), ShouldAlmostEqual, 0.5, 1e-12)
			So(cost.Impact.Sign(), ShouldEqual, 0)
			So(cost.EntryFee.Float64(), ShouldAlmostEqual, 101*0.0025, 1e-12)
			So(cost.BreakEven.Cmp(cost.EntryPrice), ShouldBeGreaterThan, 0)
			So(cost.RoundTripFees.Sign(), ShouldEqual, 1)
		})
	})

	Convey("Given executable quantity distributed across current ask depth", t, func() {
		price := entryEconomicsDepthFixture(t)
		cost, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(2))

		Convey("It should price the observed ask walk and impact", func() {
			So(err, ShouldBeNil)
			So(cost.EntryPrice.Float64(), ShouldAlmostEqual, 101.5, 1e-12)
			So(cost.BestAsk.Float64(), ShouldAlmostEqual, 101, 1e-12)
			So(cost.Impact.Float64(), ShouldAlmostEqual, 0.5, 1e-12)
		})

		Convey("It should reject quantity beyond visible asks", func() {
			_, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(6))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "visible ask depth")
		})
	})

	Convey("Given crossed current quotes", t, func() {
		price := entryEconomicsFixture(t, 99, 100, 5)
		_, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(1))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "crossed")
	})

	Convey("Given a high-priced asset with a sub-tick-sized quantity", t, func() {
		quantity := 0.00051057
		price := entryEconomicsFixture(t, 64951.1, 64951.0, quantity)
		cost, err := price.EntryCost("EDGE/USD", decimal.NewFromFloat64(quantity))
		So(err, ShouldBeNil)
		So(cost.GrossNotional.Sign(), ShouldEqual, 1)
	})
}

func TestPriceExecutableQuantity(t *testing.T) {
	Convey("Given a request and current multi-level asks", t, func() {
		price := entryEconomicsDepthFixture(t)

		Convey("It should preserve a request visible depth can fill", func() {
			quantity, err := price.ExecutableQuantity(
				"EDGE/USD", decimal.NewFromFloat64(4),
			)
			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 4, 1e-12)
		})

		Convey("It should cap only at total observable asks, not at a forecast", func() {
			quantity, err := price.ExecutableQuantity(
				"EDGE/USD", decimal.NewFromFloat64(9),
			)
			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 5, 1e-12)
		})
	})

	Convey("Given ticker quotes without a managed book", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 3)
		quantity, err := price.ExecutableQuantity(
			"EDGE/USD", decimal.NewFromFloat64(5),
		)
		So(err, ShouldBeNil)
		So(quantity.Float64(), ShouldAlmostEqual, 3, 1e-12)
	})

	Convey("Given crossed managed depth", t, func() {
		managed := entryEconomicsBook(
			t,
			bookLevel{book.Bid, 100, 5},
			bookLevel{book.Ask, 99, 5},
		)
		price := entryEconomicsManagedFixture(t, managed, 101, 100, 5)
		_, err := price.ExecutableQuantity("EDGE/USD", decimal.NewFromFloat64(1))
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "crossed visible book")
	})

	Convey("Given a non-finite or non-positive request", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 3)
		_, err := price.ExecutableQuantity("EDGE/USD", decimal.NewFromFloat64(0))
		So(err, ShouldNotBeNil)
		So(math.IsNaN(0), ShouldBeFalse)
	})
}

type bookLevel struct {
	direction book.BookDirection
	price     float64
	quantity  float64
}

func entryEconomicsDepthFixture(testingTB testing.TB) *Price {
	managed := entryEconomicsBook(
		testingTB,
		bookLevel{book.Bid, 100, 1},
		bookLevel{book.Bid, 99, 2},
		bookLevel{book.Bid, 50, 2},
		bookLevel{book.Ask, 101, 1},
		bookLevel{book.Ask, 102, 2},
		bookLevel{book.Ask, 150, 2},
	)
	return entryEconomicsManagedFixture(testingTB, managed, 101, 100, 1)
}

func entryEconomicsBook(
	testingTB testing.TB,
	levels ...bookLevel,
) *book.Book {
	testingTB.Helper()
	managed := book.New()
	managed.NoBookCrossing = false

	for _, level := range levels {
		managed.Update(&book.UpdateOptions{
			Direction: level.direction,
			Price:     decimal.NewFromFloat64(level.price),
			Quantity:  decimal.NewFromFloat64(level.quantity),
			Timestamp: time.Now(),
		})
	}

	return managed
}

func entryEconomicsManagedFixture(
	testingTB testing.TB,
	managed *book.Book,
	ask float64,
	bid float64,
	quoteQuantity float64,
) *Price {
	testingTB.Helper()

	private := &entryEconomicsBookConn{Conn: mock.NewConn(), managed: managed}
	api := websocket.NewAPI(testingTB.Context(), mock.NewConn(), private)
	price := NewPrice(api)
	price.fees.Store("EDGE/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.25),
	})
	price.Update(&kraken.TickerData{
		Symbol: "EDGE/USD",
		Ask:    decimal.NewFromFloat64(ask),
		AskQty: quoteQuantity,
		Bid:    decimal.NewFromFloat64(bid),
		BidQty: quoteQuantity,
	})

	return price
}

func entryEconomicsFixture(
	testingTB testing.TB,
	ask float64,
	bid float64,
	quoteQuantity float64,
) *Price {
	testingTB.Helper()
	price, _ := newPriceSurface(testingTB, "EDGE/USD")
	price.Update(&kraken.TickerData{
		Symbol: "EDGE/USD",
		Ask:    decimal.NewFromFloat64(ask),
		AskQty: quoteQuantity,
		Bid:    decimal.NewFromFloat64(bid),
		BidQty: quoteQuantity,
	})

	return price
}

func BenchmarkPriceEntryCost(b *testing.B) {
	price := entryEconomicsFixture(b, 64951.1, 64951.0, 0.00051057)
	quantity := decimal.NewFromFloat64(0.00051057)

	for b.Loop() {
		if _, err := price.EntryCost("EDGE/USD", quantity); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPriceExecutableQuantity(b *testing.B) {
	price := entryEconomicsDepthFixture(b)
	requested := decimal.NewFromFloat64(4)

	for b.Loop() {
		if _, err := price.ExecutableQuantity("EDGE/USD", requested); err != nil {
			b.Fatal(err)
		}
	}
}
