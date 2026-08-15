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

func TestPriceEntryEconomics(t *testing.T) {
	Convey("Given a forecast larger than the measured round-trip cost", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 5)

		Convey("It should preserve every cost in compatible midpoint-return units", func() {
			expectedReturn := math.Expm1(0.02)
			economics, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(5),
				0.02,
			)

			So(err, ShouldBeNil)
			So(economics.Midpoint.Float64(), ShouldAlmostEqual, 100.5, 1e-12)
			So(economics.ExpectedReturn.Float64(), ShouldAlmostEqual, expectedReturn, 1e-12)
			So(economics.ExpectedSpread.Float64(), ShouldAlmostEqual, 0.5/100.5, 1e-12)
			So(economics.ExpectedFees.Float64(), ShouldAlmostEqual,
				(101*0.0025+100.5*(1+expectedReturn)*0.0025)/100.5, 1e-12)
			So(economics.ExpectedImpact.Sign(), ShouldEqual, 0)
			So(economics.NetReturn.Sign(), ShouldEqual, 1)
		})
	})

	Convey("Given a positive forecast that is smaller than spread and fees", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 5)

		Convey("It should expose a negative executable return", func() {
			economics, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				0.005,
			)

			So(err, ShouldBeNil)
			So(economics.ExpectedReturn.Sign(), ShouldEqual, 1)
			So(economics.NetReturn.Sign(), ShouldEqual, -1)
		})
	})

	Convey("Given the exact analytical break-even return", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 5)
		feeRate := 0.0025
		midpoint := (101.0 + 100.0) / 2.0
		breakEvenArithmetic := 101.0*(1.0+feeRate)/(midpoint*(1.0-feeRate)) - 1.0
		breakEven := math.Log1p(breakEvenArithmetic)
		returnQuantum := math.Pow10(-decimal.DefaultScale)

		Convey("The adjacent floating-point returns should fall on opposite sides", func() {
			below, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				breakEven-returnQuantum,
			)
			So(err, ShouldBeNil)

			above, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				breakEven+returnQuantum,
			)
			So(err, ShouldBeNil)
			So(below.NetReturn.Sign(), ShouldBeLessThanOrEqualTo, 0)
			So(above.NetReturn.Sign(), ShouldBeGreaterThanOrEqualTo, 0)
			So(below.NetReturn.Cmp(above.NetReturn), ShouldBeLessThan, 0)
		})
	})

	Convey("Given a quantity at the complete best-quote boundary", t, func() {
		price := entryEconomicsFixture(t, 100.02, 100, 5)

		Convey("It should accept the boundary and refuse the next representable quantity", func() {
			_, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(5),
				0.02,
			)
			So(err, ShouldBeNil)

			_, err = price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(math.Nextafter(5, math.Inf(1))),
				0.02,
			)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "visible ask")
		})
	})

	Convey("Given executable quantity distributed across visible depth", t, func() {
		price := entryEconomicsDepthFixture(t)

		Convey("It should price the observable ask walk and expose its impact", func() {
			economics, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(2),
				0.05,
			)

			So(err, ShouldBeNil)
			So(economics.ExpectedImpact.Float64(), ShouldAlmostEqual,
				0.5/100.5, 1e-12)
			So(economics.NetReturn.Sign(), ShouldEqual, 1)
		})

		Convey("It should reject a quantity the observable asks cannot fill", func() {
			_, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(6),
				0.05,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "visible ask depth")
		})
	})

	Convey("Given identical entries but radically different current bid depth", t, func() {
		thinBids := entryEconomicsBook(
			t,
			bookLevel{book.Bid, 100, 0.01},
			bookLevel{book.Bid, 1, 1000},
			bookLevel{book.Ask, 101, 5},
		)
		deepBids := entryEconomicsBook(
			t,
			bookLevel{book.Bid, 100, 1000},
			bookLevel{book.Ask, 101, 5},
		)
		thin := entryEconomicsManagedFixture(t, thinBids, 101, 100, 5)
		deep := entryEconomicsManagedFixture(t, deepBids, 101, 100, 5)

		Convey("Current liquidation depth should not impersonate the forecast-horizon exit book", func() {
			thinEconomics, err := thin.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(3),
				0.05,
			)
			So(err, ShouldBeNil)

			deepEconomics, err := deep.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(3),
				0.05,
			)
			So(err, ShouldBeNil)
			So(thinEconomics.NetReturn.String(), ShouldEqual, deepEconomics.NetReturn.String())
			So(thinEconomics.ExpectedImpact.String(),
				ShouldEqual, deepEconomics.ExpectedImpact.String())
		})
	})

	Convey("Given a high-priced asset with a sub-tick-sized quantity", t, func() {
		quantity := 0.00051057
		price := entryEconomicsFixture(t, 64951.1, 64951.0, quantity)

		Convey("It should compare the quantity without rounding it to zero", func() {
			economics, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(quantity),
				0.01,
			)

			So(err, ShouldBeNil)
			So(economics.NetReturn.Sign(), ShouldEqual, 1)
		})
	})

	Convey("Given crossed quotes", t, func() {
		price := entryEconomicsFixture(t, 99, 100, 5)

		Convey("It should refuse to invent executable long economics", func() {
			_, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				0.02,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "crossed")
		})

		Convey("It should also reject an independently crossed managed book", func() {
			managed := entryEconomicsBook(
				t,
				bookLevel{book.Bid, 100, 5},
				bookLevel{book.Ask, 99, 5},
			)
			price = entryEconomicsManagedFixture(t, managed, 101, 100, 5)
			_, err := price.EntryEconomics(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				0.02,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "crossed visible book")
		})
	})
}

func TestPriceProfitableQuantity(t *testing.T) {
	Convey("Given a capital-sized request and a multi-level two-sided book", t, func() {
		price := entryEconomicsDepthFixture(t)

		Convey("It should preserve a request the complete book can execute", func() {
			quantity, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(2),
				0.05,
				0,
			)

			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 2, 1e-12)
		})

		Convey("It should stop before the first loss-making marginal segment", func() {
			quantity, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(5),
				0.05,
				0,
			)

			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 3, 1e-12)
		})

		Convey("It should reject a forecast that cannot pay the best segment", func() {
			_, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(2),
				0.001,
				0,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "no visible ask segment")
		})

		Convey("It should stop before a marginal ask segment dilutes regulated utility", func() {
			quantity, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(5),
				0.05,
				0.04,
			)

			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 1, 1e-12)
		})
	})

	Convey("Given valid ticker quotes without a managed book", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 3)

		Convey("It should cap the request at the quoted ask quantity", func() {
			quantity, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(5),
				0.05,
				0,
			)

			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 3, 1e-12)
		})

		Convey("It should not cap an entry at the current bid quantity", func() {
			price.Update(&kraken.TickerData{
				Symbol: "EDGE/USD",
				Ask:    decimal.NewFromFloat64(101),
				AskQty: 3,
				Bid:    decimal.NewFromFloat64(100),
				BidQty: 0.01,
			})
			quantity, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(3),
				0.05,
				0,
			)

			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 3, 1e-12)
		})

		Convey("It should report like-for-like prices when the forecast cannot clear costs", func() {
			_, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				0.001,
				0,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "0.001001 arithmetic return")
			So(err.Error(), ShouldContainSubstring, "exit value")
			So(err.Error(), ShouldContainSubstring, "entry value")
			So(err.Error(), ShouldContainSubstring, "executable quantity 0")
			So(err.Error(), ShouldNotContainSubstring, "%!")
		})
	})

	Convey("Given sufficient asks and almost no current bid depth", t, func() {
		managed := entryEconomicsBook(
			t,
			bookLevel{book.Bid, 100, 0.01},
			bookLevel{book.Ask, 101, 3},
		)
		price := entryEconomicsManagedFixture(t, managed, 101, 100, 3)

		Convey("It should size the entry from executable asks rather than a fictional future bid walk", func() {
			quantity, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(3),
				0.05,
				0,
			)

			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 3, 1e-12)
		})
	})

	Convey("Given the observed SYND rally quotes and active taker fee", t, func() {
		forecast := 0.03128988368713027
		price := entryEconomicsFixture(t, 0.01586, 0.01585, 5000)
		price.fees.Store("EDGE/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.8),
		})

		Convey("It should admit the tight book that preceded the next rally leg", func() {
			quantity, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(5000),
				forecast,
				0,
			)

			So(err, ShouldBeNil)
			So(quantity.Float64(), ShouldAlmostEqual, 5000, 1e-12)
		})

		Convey("It should reject the post-sweep book whose spread exceeded the forecast", func() {
			price.Update(&kraken.TickerData{
				Symbol: "EDGE/USD",
				Ask:    decimal.NewFromFloat64(0.01839),
				AskQty: 5000,
				Bid:    decimal.NewFromFloat64(0.01592),
				BidQty: 5000,
			})
			_, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(5000),
				forecast,
				0,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "observable entry costs do not clear")
		})
	})

	Convey("Given invalid execution domains", t, func() {
		Convey("A crossed ticker should be rejected", func() {
			price := entryEconomicsFixture(t, 99, 100, 3)
			_, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				0.05,
				0,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "crossed")
		})

		Convey("A crossed managed book should be rejected", func() {
			managed := entryEconomicsBook(
				t,
				bookLevel{book.Bid, 100, 3},
				bookLevel{book.Ask, 99, 3},
			)
			price := entryEconomicsManagedFixture(t, managed, 101, 100, 3)
			_, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				0.05,
				0,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "crossed visible book")
		})

		Convey("A fee at the entire notional should be rejected", func() {
			price := entryEconomicsFixture(t, 101, 100, 3)
			price.fees.Store("EDGE/USD", kraken.TradeVolumeFee{
				Fee: decimal.NewFromInt64(100),
			})
			_, err := price.ProfitableQuantity(
				"EDGE/USD",
				decimal.NewFromFloat64(1),
				0.05,
				0,
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "valid taker fee")
		})

		Convey("A non-finite forecast should fail loudly", func() {
			price := entryEconomicsFixture(t, 101, 100, 3)

			So(func() {
				_, _ = price.ProfitableQuantity(
					"EDGE/USD",
					decimal.NewFromFloat64(1),
					math.NaN(),
					0,
				)
			}, ShouldPanic)
		})

		Convey("A non-finite ticker quantity should fail loudly", func() {
			price := entryEconomicsFixture(t, 101, 100, math.NaN())

			So(func() {
				_, _ = price.ProfitableQuantity(
					"EDGE/USD",
					decimal.NewFromFloat64(1),
					0.05,
					0,
				)
			}, ShouldPanic)
		})
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

func BenchmarkPriceEntryEconomics(b *testing.B) {
	price := entryEconomicsFixture(b, 64951.1, 64951.0, 0.00051057)
	quantity := decimal.NewFromFloat64(0.00051057)

	for b.Loop() {
		if _, err := price.EntryEconomics("EDGE/USD", quantity, 0.01); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPriceProfitableQuantity(b *testing.B) {
	price := entryEconomicsDepthFixture(b)
	requested := decimal.NewFromFloat64(4)

	b.Run("Accepted", func(b *testing.B) {
		for b.Loop() {
			if _, err := price.ProfitableQuantity(
				"EDGE/USD", requested, 0.05, 0,
			); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Rejected", func(b *testing.B) {
		for b.Loop() {
			if _, err := price.ProfitableQuantity(
				"EDGE/USD", requested, 0.001, 0,
			); err == nil {
				b.Fatal("expected unprofitable quantity rejection")
			}
		}
	})
}
