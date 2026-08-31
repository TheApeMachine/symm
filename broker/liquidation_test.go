package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func liquidationFee() *kraken.TradeVolumeFee {
	return &kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.25)}
}

func mustDecimalOrder(orderID, price, qty string) kraken.Level3Order {
	return kraken.Level3Order{
		OrderID:    orderID,
		LimitPrice: mustDecimal(price),
		OrderQty:   mustDecimal(qty),
	}
}

func TestLiquidationSurface(t *testing.T) {
	Convey("Given a bounded liquidation reducer with no state", t, func() {
		book := newLiquidationBook("EDGE/USD")

		surface := book.Surface(
			decimal.NewFromFloat64(100000),
			nil,
			liquidationFee(),
			time.Now(),
		)

		Convey("the book reports incomplete and never fabricates a fill", func() {
			So(surface.Symbol, ShouldEqual, "EDGE/USD")
			So(surface.BookComplete, ShouldBeFalse)
			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.ExecutableVWAP, ShouldBeNil)
			So(surface.ExecutableValue, ShouldBeNil)
		})
	})

	Convey("Given a coherent two-sided snapshot", t, func() {
		book := newLiquidationBook("EDGE/USD")

		Convey("an exact full-lot fill at one level yields the gross VWAP and fee-net value", func() {
			book.Apply(kraken.Level3Data{
				Symbol: "EDGE/USD",
				Type:   "snapshot",
				Bids:   []kraken.Level3Order{mustDecimalOrder("b1", "100", "1000")},
				Asks:   []kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
			})

			surface := book.Surface(
				decimal.NewFromFloat64(1000),
				decimal.NewFromFloat64(90),
				liquidationFee(),
				time.Now(),
			)

			So(surface.BookComplete, ShouldBeTrue)
			So(surface.FullyExecutable, ShouldBeTrue)
			So(surface.ExecutableQty.Cmp(decimal.NewFromFloat64(1000)), ShouldEqual, 0)
			So(surface.FloorCoverageQty.Cmp(decimal.NewFromFloat64(1000)), ShouldEqual, 0)
			So(surface.ExecutableVWAP.Cmp(decimal.NewFromFloat64(100)), ShouldEqual, 0)

			// gross 100*1000 = 100000; fee 0.25% -> 250; net 99750
			So(surface.ExecutableValue.Cmp(mustDecimal("99750")), ShouldEqual, 0)
		})

		Convey("a fill spanning multiple bid levels computes the true VWAP", func() {
			book.Apply(kraken.Level3Data{
				Symbol: "EDGE/USD",
				Type:   "snapshot",
				Bids: []kraken.Level3Order{
					mustDecimalOrder("b1", "100", "500"),
					mustDecimalOrder("b2", "99", "500"),
				},
				Asks: []kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
			})

			surface := book.Surface(
				decimal.NewFromFloat64(1000),
				nil,
				liquidationFee(),
				time.Now(),
			)

			So(surface.FullyExecutable, ShouldBeTrue)
			// VWAP = (100*500 + 99*500) / 1000 = 99.5
			So(surface.ExecutableVWAP.Cmp(mustDecimal("99.5")), ShouldEqual, 0)
			// gross = 99500; fee 0.25% -> 248.75; net 99251.25
			So(surface.ExecutableValue.Cmp(mustDecimal("99251.25")), ShouldEqual, 0)
		})

		Convey("a shallow book cannot complete the lot and reports no VWAP", func() {
			book.Apply(kraken.Level3Data{
				Symbol: "EDGE/USD",
				Type:   "snapshot",
				Bids:   []kraken.Level3Order{mustDecimalOrder("b1", "100", "100")},
				Asks:   []kraken.Level3Order{mustDecimalOrder("a1", "101", "100")},
			})

			surface := book.Surface(
				decimal.NewFromFloat64(1000),
				nil,
				liquidationFee(),
				time.Now(),
			)

			So(surface.BookComplete, ShouldBeTrue)
			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.ExecutableQty.Cmp(decimal.NewFromFloat64(100)), ShouldEqual, 0)
			So(surface.ExecutableVWAP, ShouldBeNil)
			So(surface.ExecutableValue, ShouldBeNil)
		})

		Convey("floor coverage smaller than SellableQty is reported independently", func() {
			book.Apply(kraken.Level3Data{
				Symbol: "EDGE/USD",
				Type:   "snapshot",
				Bids: []kraken.Level3Order{
					mustDecimalOrder("b1", "100", "800"),
					mustDecimalOrder("b2", "95", "500"),
				},
				Asks: []kraken.Level3Order{mustDecimalOrder("a1", "101", "2000")},
			})

			surface := book.Surface(
				decimal.NewFromFloat64(1000),
				decimal.NewFromFloat64(97),
				liquidationFee(),
				time.Now(),
			)

			// coverable at or above 97 = only the 800 at 100.
			So(surface.FloorCoverageQty.Cmp(decimal.NewFromFloat64(800)), ShouldEqual, 0)
			So(surface.FullyExecutable, ShouldBeTrue)
		})
	})
}

func TestLiquidationIncremental(t *testing.T) {
	Convey("Given a seeded liquidation book", t, func() {
		book := newLiquidationBook("EDGE/USD")
		book.Apply(kraken.Level3Data{
			Symbol: "EDGE/USD",
			Type:   "snapshot",
			Bids: []kraken.Level3Order{
				mustDecimalOrder("b1", "100", "500"),
			},
			Asks: []kraken.Level3Order{
				mustDecimalOrder("a1", "101", "1000"),
			},
		})

		Convey("incremental add/modify/delete changes the executable mark correctly", func() {
			// Add a better bid, keeping the ask above it for coherence.
			book.Apply(kraken.Level3Data{
				Symbol: "EDGE/USD", Type: "update",
				Bids: []kraken.Level3Order{mustDecimalOrder("b2", "101", "500")},
				Asks: []kraken.Level3Order{mustDecimalOrder("a1", "102", "1000")},
			})

			surface := book.Surface(
				decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
			)

			So(surface.FullyExecutable, ShouldBeTrue)
			// (101*500 + 100*500)/1000 = 100.5
			So(surface.ExecutableVWAP.Cmp(mustDecimal("100.5")), ShouldEqual, 0)

			// Modify b2 upward.
			book.Apply(kraken.Level3Data{
				Symbol: "EDGE/USD", Type: "update",
				Bids: []kraken.Level3Order{mustDecimalOrder("b2", "102", "500")},
				Asks: []kraken.Level3Order{mustDecimalOrder("a1", "103", "1000")},
			})

			surface = book.Surface(
				decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
			)

			// (102*500 + 100*500)/1000 = 101
			So(surface.ExecutableVWAP.Cmp(mustDecimal("101")), ShouldEqual, 0)

			// Delete b2: back to a single 100 bid.
			book.Apply(kraken.Level3Data{
				Symbol: "EDGE/USD", Type: "update",
				Bids: []kraken.Level3Order{{Event: "delete", OrderID: "b2"}},
			})

			surface = book.Surface(
				decimal.NewFromFloat64(1000), nil, liquidationFee(), time.Now(),
			)

			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.BookComplete, ShouldBeTrue)
		})

		Convey("a crossed update after a valid state invalidates the book", func() {
			book.Apply(kraken.Level3Data{
				Symbol: "EDGE/USD", Type: "update",
				Bids: []kraken.Level3Order{mustDecimalOrder("b1", "102", "500")},
				Asks: []kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
			})

			surface := book.Surface(
				decimal.NewFromFloat64(500), nil, liquidationFee(), time.Now(),
			)

			So(surface.BookComplete, ShouldBeFalse)
			So(surface.ExecutableVWAP, ShouldBeNil)
		})
	})
}

func BenchmarkLiquidationSurface(b *testing.B) {
	book := newLiquidationBook("EDGE/USD")
	book.Apply(kraken.Level3Data{
		Symbol: "EDGE/USD",
		Type:   "snapshot",
		Bids: []kraken.Level3Order{
			mustDecimalOrder("b1", "100", "40000"),
			mustDecimalOrder("b2", "99", "30000"),
			mustDecimalOrder("b3", "98", "30000"),
		},
		Asks: []kraken.Level3Order{mustDecimalOrder("a1", "101", "100000")},
	})
	sellable := decimal.NewFromFloat64(100000)
	floor := decimal.NewFromFloat64(51)
	at := time.Now()
	fee := liquidationFee()
	b.ReportAllocs()

	for b.Loop() {
		book.Surface(sellable, floor, fee, at)
	}
}

func BenchmarkLiquidationSurfaceSmallLot(b *testing.B) {
	book := newLiquidationBook("EDGE/USD")
	book.Apply(kraken.Level3Data{
		Symbol: "EDGE/USD",
		Type:   "snapshot",
		Bids:   []kraken.Level3Order{mustDecimalOrder("b1", "64", "100000")},
		Asks:   []kraken.Level3Order{mustDecimalOrder("a1", "65", "100000")},
	})
	sellable := decimal.NewFromFloat64(1)
	at := time.Now()
	fee := liquidationFee()
	b.ReportAllocs()

	for b.Loop() {
		book.Surface(sellable, nil, fee, at)
	}
}

func BenchmarkLiquidationSurfaceDeepBook(b *testing.B) {
	book := newLiquidationBook("EDGE/USD")

	bids := make([]kraken.Level3Order, 0, 200)
	asks := make([]kraken.Level3Order, 0, 200)

	for level := 0; level < 200; level++ {
		bidPrice := 200 - level
		bids = append(bids, mustDecimalOrder(
			"b"+decimal.NewFromInt64(int64(level)).String(),
			decimal.NewFromInt64(int64(bidPrice)).String(),
			"100",
		))
		asks = append(asks, mustDecimalOrder(
			"a"+decimal.NewFromInt64(int64(level)).String(),
			decimal.NewFromInt64(int64(201+level)).String(),
			"100",
		))
	}

	book.Apply(kraken.Level3Data{
		Symbol: "EDGE/USD",
		Type:   "snapshot",
		Bids:   bids,
		Asks:   asks,
	})
	sellable := decimal.NewFromFloat64(10000)
	at := time.Now()
	fee := liquidationFee()
	b.ReportAllocs()

	for b.Loop() {
		book.Surface(sellable, nil, fee, at)
	}
}

func TestLiquidationBoundedState(t *testing.T) {
	Convey("Given a liquidation reducer under a long stream", t, func() {
		book := newLiquidationBook("EDGE/USD")

		for index := 0; index < 400; index++ {
			orderID := "b" + decimal.NewFromInt64(int64(index%5)).String()
			book.Apply(kraken.Level3Data{
				Symbol: "EDGE/USD", Type: "update",
				Bids: []kraken.Level3Order{
					mustDecimalOrder(orderID, "100", "10"),
				},
				Asks: []kraken.Level3Order{
					mustDecimalOrder("a1", "101", "10"),
				},
			})
		}

		Convey("state is bounded by the distinct order identities, not the stream length", func() {
			book.mu.RLock()
			bids := len(book.bids)
			asks := len(book.asks)
			book.mu.RUnlock()

			So(bids, ShouldBeLessThan, 400)
			So(bids, ShouldEqual, 5)
			So(asks, ShouldBeLessThan, 400)
		})
	})
}

func TestDeskLevel3NoLifecycleAllocatesNothing(t *testing.T) {
	Convey("Given a desk with no position for a symbol", t, func() {
		conn := newExecutingConn(nil)
		desk, workload := newDeliveryDesk(t, conn)
		conn.workload = workload

		Convey("incoming L3 traffic for that symbol never allocates book state", func() {
			So(
				desk.StepLevel3(coherentL3("NO_POSITION/USD", "2.50", "2.51")),
				ShouldBeNil,
			)

			_, found := desk.positions.Load("NO_POSITION/USD")
			So(found, ShouldBeFalse)
		})
	})
}

func TestLiquidationRace(t *testing.T) {
	Convey("Given concurrent ingress and surface reads on one reducer", t, func() {
		book := newLiquidationBook("EDGE/USD")
		book.Apply(kraken.Level3Data{
			Symbol: "EDGE/USD",
			Type:   "snapshot",
			Bids:   []kraken.Level3Order{mustDecimalOrder("b1", "100", "1000")},
			Asks:   []kraken.Level3Order{mustDecimalOrder("a1", "101", "1000")},
		})

		done := make(chan struct{})

		go func() {
			defer close(done)

			for index := 0; index < 1000; index++ {
				book.Apply(kraken.Level3Data{
					Symbol: "EDGE/USD", Type: "update",
					Bids: []kraken.Level3Order{
						mustDecimalOrder("b1", "99", "1000"),
						mustDecimalOrder("b2", "100", "1000"),
					},
					Asks: []kraken.Level3Order{
						mustDecimalOrder("a1", "101", "1000"),
					},
				})
			}
		}()

		Convey("surface reads observe coherent committed states without panic", func() {
			for index := 0; index < 1000; index++ {
				_ = book.Surface(
					decimal.NewFromFloat64(1000),
					nil,
					liquidationFee(),
					time.Now(),
				)
			}
		})

		<-done
	})
}
