package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
newPriceSurface creates a price surface with the symbol's executable fee row.
*/
func newPriceSurface(t testing.TB, symbol string) (*Price, *websocket.API) {
	t.Helper()
	api := websocket.NewAPI(t.Context(), newMockConn(), newMockConn())
	price := newTestPrice(t, api)
	price.fees.Store(symbol, kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.25),
	})

	return price, api
}

/*
newTestPrice builds a Price directly from an api. recorder is optional and
left nil for tests that don't need audit capture.
*/
func newTestPrice(t testing.TB, api *websocket.API) *Price {
	t.Helper()

	return NewPrice(api, nil)
}

/*
newQuantityPrice creates the executable BTC/USD quantity fixture.
*/
func newQuantityPrice(t testing.TB) *Price {
	t.Helper()
	price, api := newPriceSurface(t, "BTC/USD")
	api.Normalizer().Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "BTC", Decimals: 8, DisplayDecimals: 8},
			"USD": {AltName: "USD", Decimals: 2, DisplayDecimals: 2},
		},
		NewPairs: map[string]spot.AssetPair{
			"BTCUSD": {
				WSName: "BTC/USD", Base: "BTC", Quote: "USD",
				PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1,
			},
		},
	})
	price.Update(&kraken.TickerData{
		Symbol: "BTC/USD",
		Ask:    decimal.NewFromFloat64(100000),
		Bid:    decimal.NewFromFloat64(99900),
	})

	return price
}

func TestPriceUpdate(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST1")

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST1",
				Ask:    decimal.NewFromFloat64(30000.00),
				Bid:    decimal.NewFromFloat64(29950.00),
			}

			Convey("When the price surface is updated", func() {
				price.Update(ticker)

				Convey("It should store the new ticker data in the cache", func() {
					So(price.Tick("TEST1"), ShouldResemble, ticker)
				})
			})
		})
	})
}

func TestPriceMark(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST2")

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST2",
				Ask:    decimal.NewFromFloat64(40000.00),
				Bid:    decimal.NewFromFloat64(39950.00),
			}

			price.Update(ticker)

			Convey("When the mark price is requested for buying", func() {
				markPrice := price.Mark("TEST2", BUY)

				Convey("It should return the ask with the taker fee", func() {
					So(markPrice.Float64(), ShouldAlmostEqual, 40100, 1e-12)
				})
			})

			Convey("When the mark price is requested for selling", func() {
				markPrice := price.Mark("TEST2", SELL)

				Convey("It should return the bid after the taker fee", func() {
					So(markPrice.Float64(), ShouldAlmostEqual, 39850.125, 1e-12)
				})
			})
		})
	})
}

func TestPricePnL(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST3")
		pair := kraken.InstrumentPair{Symbol: "TEST3", Base: "BTC", Quote: "USD"}
		holding := &types.Holding{
			Symbol:     "TEST3",
			Qty:        decimal.NewFromFloat64(1.0),
			EntryPrice: decimal.NewFromFloat64(45000.00),
			EntryFee:   decimal.NewFromInt64(0),
		}

		Convey("Given an authoritative economic mark", func() {
			mark := decimal.NewFromFloat64(49950.00)
			holding.Mark = mark

			Convey("When the PnL is calculated for a holding", func() {
				pnl := price.PnL(pair, holding)

				Convey("It should return the profit or loss based on the authoritative mark, including fees", func() {
					// mark * qty * (1 - exitFee) - entryPrice * qty - entryFee
					So(pnl.Float64(), ShouldAlmostEqual, 4825.125, 1e-12)
				})
			})
		})
	})

	Convey("Given a holding before its mark is set", t, func() {
		price, _ := newPriceSurface(t, "COLD/USD")
		holding := &types.Holding{
			Qty:        decimal.NewFromFloat64(1),
			EntryPrice: decimal.NewFromFloat64(100),
			EntryFee:   decimal.NewFromInt64(0),
		}

		Convey("It should reject the incomplete valuation without dereferencing it", func() {
			So(func() {
				So(price.PnL(kraken.InstrumentPair{Symbol: "COLD/USD"}, holding),
					ShouldBeNil)
			}, ShouldNotPanic)
		})
	})

	Convey("Given a tiny quantity of a high-priced asset", t, func() {
		price, _ := newPriceSurface(t, "PAXG/USD")
		entryPrice := 4339.01
		mark := 4338.33
		quantity := 0.00765083
		entryFee := entryPrice * quantity * 0.0025
		pair := kraken.InstrumentPair{
			Symbol: "PAXG/USD",
			Base:   "PAXG",
			Quote:  "USD",
		}
		holding := &types.Holding{
			Symbol:     "PAXG/USD",
			Qty:        decimal.NewFromFloat64(quantity),
			EntryPrice: decimal.NewFromFloat64(entryPrice),
			EntryFee:   decimal.NewFromFloat64(entryFee),
			Mark:       decimal.NewFromFloat64(mark),
		}

		Convey("It should multiply price and quantity without rounding quantity to a cent", func() {
			pnl := price.PnL(pair, holding)
			expected := mark*quantity*(1-0.0025) -
				entryPrice*quantity - entryFee

			So(pnl.Sign(), ShouldEqual, -1)
			So(pnl.Float64(), ShouldAlmostEqual, expected, 1e-10)
			So(pnl.Float64(), ShouldBeGreaterThan, -1)
		})
	})
}

func TestExitValue(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST4")
		pair := kraken.InstrumentPair{Symbol: "TEST4", Base: "BTC", Quote: "USD"}
		holding := &types.Holding{
			Symbol:     "TEST4",
			Qty:        decimal.NewFromFloat64(2.0),
			EntryPrice: decimal.NewFromFloat64(60000.00),
			EntryFee:   decimal.NewFromInt64(0),
		}

		Convey("Given an authoritative economic mark", func() {
			holding.Mark = decimal.NewFromFloat64(64950.00)

			Convey("When the exit value is calculated for a holding", func() {
				exitValue := price.ExitValue(pair, holding)

				Convey("It should return the exit value based on the authoritative mark, fee-net", func() {
					So(exitValue.Float64(), ShouldAlmostEqual, 129575.25, 1e-12)
				})
			})
		})
	})

	Convey("Given no holding or mark", t, func() {
		price, _ := newPriceSurface(t, "COLD/USD")

		Convey("It should reject both incomplete domains without dereferencing them", func() {
			So(func() {
				So(price.ExitValue(kraken.InstrumentPair{Symbol: "COLD/USD"}, nil),
					ShouldBeNil)
			}, ShouldNotPanic)
		})
	})
}

func TestPriceWithFriction(t *testing.T) {
	Convey("Given no authoritative exit book", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 3)
		holding := &types.Holding{Qty: decimal.NewFromFloat64(1)}

		Convey("It should return an explicit error instead of an unexplained nil value, since no full-depth book is ever available", func() {
			adjusted, err := price.WithFriction(
				kraken.InstrumentPair{Symbol: "EDGE/USD"},
				holding,
				decimal.NewFromFloat64(10),
			)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "visible bid book required")
			So(adjusted, ShouldBeNil)
		})
	})

	Convey("Given a well-formed holding, ticker, and fee row", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 3)
		holding := &types.Holding{Qty: decimal.NewFromFloat64(2)}

		Convey("It should still error, since ExecutableSurface/WithFriction never fabricate a book-derived adjustment", func() {
			adjusted, err := price.WithFriction(
				kraken.InstrumentPair{Symbol: "EDGE/USD"},
				holding,
				decimal.NewFromFloat64(10),
			)

			So(err, ShouldNotBeNil)
			So(adjusted, ShouldBeNil)
		})
	})

	Convey("Given a nil holding", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 3)

		Convey("It should reject the incomplete request before ever reaching the book question", func() {
			adjusted, err := price.WithFriction(
				kraken.InstrumentPair{Symbol: "EDGE/USD"},
				nil,
				decimal.NewFromFloat64(10),
			)

			So(err, ShouldNotBeNil)
			So(adjusted, ShouldBeNil)
		})
	})
}

func TestPriceTick(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST7")

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST7",
				Ask:    decimal.NewFromFloat64(80000.00),
				Bid:    decimal.NewFromFloat64(79950.00),
			}

			price.Update(ticker)

			Convey("When the tick is requested for a symbol", func() {
				tick := price.Tick("TEST7")

				Convey("It should return the latest ticker data for that symbol", func() {
					So(tick, ShouldResemble, ticker)
				})
			})
		})
	})
}

func TestPriceQuantity(t *testing.T) {
	Convey("Given an integer cash balance and a fractional allocation", t, func() {
		price := newQuantityPrice(t)
		fraction := decimal.NewFromFloat64(0.20)
		notional := decimal.NewFromInt64(100).Mul(fraction)

		Convey("When the allocated cash is converted at the fee-adjusted ask", func() {
			quantity := price.Quantity("BTC/USD", notional)

			Convey("Then receiver precision should not erase the order quantity", func() {
				So(quantity, ShouldNotBeNil)
				So(quantity.Sign(), ShouldEqual, 1)
				So(quantity.String(), ShouldEqual, "0.00019950")
			})
		})
	})

	Convey("Given a notional before its first ticker arrives", t, func() {
		price, _ := newPriceSurface(t, "COLD/USD")

		Convey("It should reject the incomplete quote without dereferencing it", func() {
			So(func() {
				So(price.Quantity("COLD/USD", decimal.NewFromInt64(10)), ShouldBeNil)
			}, ShouldNotPanic)
		})
	})
}

func TestPriceReturnPct(t *testing.T) {
	Convey("Given the high-price tiny-quantity precision boundary", t, func() {
		price, _ := newPriceSurface(t, "BTC/USD")
		entryPrice := 64951.1
		bid := 64951.0
		quantity := 0.00051057
		entryFee := entryPrice * quantity * 0.0025
		pair := kraken.InstrumentPair{
			Symbol: "BTC/USD",
			Base:   "BTC",
			Quote:  "USD",
		}
		holding := &types.Holding{
			Symbol:     "BTC/USD",
			Qty:        decimal.NewFromFloat64(quantity),
			EntryPrice: decimal.NewFromFloat64(entryPrice),
			EntryFee:   decimal.NewFromFloat64(entryFee),
			Mark:       decimal.NewFromFloat64(bid),
		}

		Convey("It should report percent once instead of erasing the exit value", func() {
			entryValue := entryPrice*quantity + entryFee
			expectedPnL := bid*quantity*(1-0.0025) - entryValue
			expectedReturn := expectedPnL / entryValue * 100
			actual := price.ReturnPct(pair, holding)

			So(actual, ShouldAlmostEqual, expectedReturn, 1e-10)
			So(actual, ShouldBeGreaterThan, -1)
			So(actual, ShouldBeLessThan, 0)
		})
	})
}

/*
TestPriceExecutableSurface asserts the real executable-liquidation contract:
ExecutableSurface walks the authoritative L3 book's visible bid depth (already
best-first) to derive BookComplete, ExecutableQty, FloorCoverageQty,
FullyExecutable, ExecutableVWAP, and fee-net ExecutableValue — with no ticker
fallback and no fabricated fill when the book is missing, crossed, or shallow.
*/
func TestPriceExecutableSurface(t *testing.T) {
	Convey("Given a price surface with no book for the symbol", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 100000)

		Convey("it reports the book as incomplete, never a fabricated fill", func() {
			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(100000),
				nil,
				time.Now(),
			)

			So(surface.Symbol, ShouldEqual, "EDGE/USD")
			So(surface.BookComplete, ShouldBeFalse)
			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.ExecutableVWAP, ShouldBeNil)
			So(surface.ExecutableValue, ShouldBeNil)
		})
	})

	Convey("Given a coherent two-sided L3 book", t, func() {
		price, _ := newPriceSurface(t, "EDGE/USD")

		Convey("an exact full-lot fill at one level yields the book's gross VWAP and fee-net value", func() {
			price.ApplyLevel3(kraken.Level3Data{
				Symbol: "EDGE/USD",
				Type:   "snapshot",
				Bids: []kraken.Level3Order{
					mustDecimalOrder("b1", "100", "1000"),
				},
				Asks: []kraken.Level3Order{
					mustDecimalOrder("a1", "101", "1000"),
				},
			})

			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(1000),
				decimal.NewFromFloat64(90),
				time.Now(),
			)

			So(surface.BookComplete, ShouldBeTrue)
			So(surface.FullyExecutable, ShouldBeTrue)
			So(surface.ExecutableQty.Cmp(decimal.NewFromFloat64(1000)), ShouldEqual, 0)
			So(surface.FloorCoverageQty.Cmp(decimal.NewFromFloat64(1000)), ShouldEqual, 0)
			So(surface.ExecutableVWAP.Cmp(decimal.NewFromFloat64(100)), ShouldEqual, 0)

			// gross proceeds 100*1000 = 100000; fee 0.25% → 250; net 99750
			So(surface.ExecutableValue.Cmp(mustDecimal("99750")), ShouldEqual, 0)
		})

		Convey("an exact fill spanning multiple bid levels computes the true VWAP", func() {
			price.ApplyLevel3(kraken.Level3Data{
				Symbol: "EDGE/USD",
				Type:   "snapshot",
				Bids: []kraken.Level3Order{
					mustDecimalOrder("b1", "100", "500"),
					mustDecimalOrder("b2", "99", "500"),
				},
				Asks: []kraken.Level3Order{
					mustDecimalOrder("a1", "101", "1000"),
				},
			})

			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(1000),
				nil,
				time.Now(),
			)

			So(surface.FullyExecutable, ShouldBeTrue)
			// VWAP = (100*500 + 99*500) / 1000 = 99.5
			So(surface.ExecutableVWAP.Cmp(mustDecimal("99.5")), ShouldEqual, 0)
			// gross = 99500; fee 0.25% → 248.75; net 99251.25
			So(surface.ExecutableValue.Cmp(mustDecimal("99251.25")), ShouldEqual, 0)
		})

		Convey("a shallow book cannot complete the lot and reports no VWAP", func() {
			price.ApplyLevel3(kraken.Level3Data{
				Symbol: "EDGE/USD",
				Type:   "snapshot",
				Bids: []kraken.Level3Order{
					mustDecimalOrder("b1", "100", "100"),
				},
				Asks: []kraken.Level3Order{
					mustDecimalOrder("a1", "101", "100"),
				},
			})

			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(1000),
				nil,
				time.Now(),
			)

			So(surface.BookComplete, ShouldBeTrue)
			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.ExecutableQty.Cmp(decimal.NewFromFloat64(100)), ShouldEqual, 0)
			So(surface.ExecutableVWAP, ShouldBeNil)
			So(surface.ExecutableValue, ShouldBeNil)
		})

		Convey("floor coverage smaller than SellableQty is reported independently", func() {
			price.ApplyLevel3(kraken.Level3Data{
				Symbol: "EDGE/USD",
				Type:   "snapshot",
				Bids: []kraken.Level3Order{
					mustDecimalOrder("b1", "100", "800"),
					mustDecimalOrder("b2", "95", "500"),
				},
				Asks: []kraken.Level3Order{
					mustDecimalOrder("a1", "101", "2000"),
				},
			})

			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(1000),
				decimal.NewFromFloat64(97),
				time.Now(),
			)

			// coverable at or above 97 = only the 800 at 100.
			So(surface.FloorCoverageQty.Cmp(decimal.NewFromFloat64(800)), ShouldEqual, 0)
			So(surface.FullyExecutable, ShouldBeTrue)
		})

		Convey("a tiny fixed-point quantity keeps its decimal precision", func() {
			price.ApplyLevel3(kraken.Level3Data{
				Symbol: "EDGE/USD",
				Type:   "snapshot",
				Bids: []kraken.Level3Order{
					mustDecimalOrder("b1", "65000", "0.00051057"),
				},
				Asks: []kraken.Level3Order{
					mustDecimalOrder("a1", "65001", "1"),
				},
			})

			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(0.00051057),
				nil,
				time.Now(),
			)

			So(surface.FullyExecutable, ShouldBeTrue)
			So(surface.ExecutableQty.Cmp(mustDecimal("0.00051057")), ShouldEqual, 0)
			So(surface.ExecutableVWAP.Cmp(decimal.NewFromFloat64(65000)), ShouldEqual, 0)
		})
	})
}

func mustDecimalOrder(orderID, price, qty string) kraken.Level3Order {
	return kraken.Level3Order{
		OrderID:    orderID,
		LimitPrice: mustDecimal(price),
		OrderQty:   mustDecimal(qty),
	}
}

func BenchmarkPriceExecutableSurface(b *testing.B) {
	price := entryEconomicsFixture(b, 101, 100, 100000)
	price.ApplyLevel3(kraken.Level3Data{
		Symbol: "EDGE/USD",
		Type:   "snapshot",
		Bids: []kraken.Level3Order{
			mustDecimalOrder("b1", "100", "40000"),
			mustDecimalOrder("b2", "99", "30000"),
			mustDecimalOrder("b3", "98", "30000"),
		},
		Asks: []kraken.Level3Order{
			mustDecimalOrder("a1", "101", "100000"),
		},
	})
	sellable := decimal.NewFromFloat64(100000)
	floor := decimal.NewFromFloat64(51)
	at := time.Now()
	b.ReportAllocs()

	for b.Loop() {
		price.ExecutableSurface("EDGE/USD", sellable, floor, at)
	}
}

/*
BenchmarkPriceExecutableSurfaceSmallLot measures a small ($10–$100) notional
liquidation that fills entirely at the touch, the common fast path.
*/
func BenchmarkPriceExecutableSurfaceSmallLot(b *testing.B) {
	price := entryEconomicsFixture(b, 65, 64, 100000)
	price.ApplyLevel3(kraken.Level3Data{
		Symbol: "EDGE/USD",
		Type:   "snapshot",
		Bids: []kraken.Level3Order{
			mustDecimalOrder("b1", "64", "100000"),
		},
		Asks: []kraken.Level3Order{
			mustDecimalOrder("a1", "65", "100000"),
		},
	})
	sellable := decimal.NewFromFloat64(1)
	at := time.Now()
	b.ReportAllocs()

	for b.Loop() {
		price.ExecutableSurface("EDGE/USD", sellable, nil, at)
	}
}

/*
BenchmarkPriceExecutableSurfaceDeepBook measures evaluation against a deep,
many-level book so the consumed-level-count cost is observable.
*/
func BenchmarkPriceExecutableSurfaceDeepBook(b *testing.B) {
	price := entryEconomicsFixture(b, 201, 200, 100000)

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

	price.ApplyLevel3(kraken.Level3Data{
		Symbol: "EDGE/USD",
		Type:   "snapshot",
		Bids:   bids,
		Asks:   asks,
	})
	sellable := decimal.NewFromFloat64(10000)
	at := time.Now()
	b.ReportAllocs()

	for b.Loop() {
		price.ExecutableSurface("EDGE/USD", sellable, nil, at)
	}
}

func BenchmarkPriceUpdate(b *testing.B) {
	price, _ := newPriceSurface(b, "TEST8")
	ticker := &kraken.TickerData{
		Symbol: "TEST8",
		Ask:    decimal.NewFromFloat64(90000.00),
		Bid:    decimal.NewFromFloat64(89950.00),
	}

	b.ResetTimer()

	for b.Loop() {
		price.Update(ticker)
	}
}

func BenchmarkPriceMark(b *testing.B) {
	price, _ := newPriceSurface(b, "TEST9")

	ticker := &kraken.TickerData{
		Symbol: "TEST9",
		Ask:    decimal.NewFromFloat64(100000.00),
		Bid:    decimal.NewFromFloat64(99950.00),
	}

	price.Update(ticker)

	b.ResetTimer()

	for b.Loop() {
		price.Mark("TEST9", BUY)
	}
}

func BenchmarkPricePnL(b *testing.B) {
	price, _ := newPriceSurface(b, "TEST10")
	pair := kraken.InstrumentPair{Symbol: "TEST10", Base: "BTC", Quote: "USD"}
	holding := &types.Holding{
		Symbol:     "TEST10",
		Qty:        decimal.NewFromFloat64(1.0),
		EntryPrice: decimal.NewFromFloat64(110000.00),
		EntryFee:   decimal.NewFromInt64(0),
	}

	ticker := &kraken.TickerData{
		Symbol: "TEST10",
		Ask:    decimal.NewFromFloat64(115000.00),
		Bid:    decimal.NewFromFloat64(114950.00),
	}

	price.Update(ticker)

	b.ResetTimer()

	for b.Loop() {
		price.PnL(pair, holding)
	}
}

func BenchmarkPriceExitValue(b *testing.B) {
	price, _ := newPriceSurface(b, "TEST11")
	pair := kraken.InstrumentPair{Symbol: "TEST11", Base: "BTC", Quote: "USD"}
	holding := &types.Holding{
		Symbol:     "TEST11",
		Qty:        decimal.NewFromFloat64(2.0),
		EntryPrice: decimal.NewFromFloat64(120000.00),
		EntryFee:   decimal.NewFromInt64(0),
	}

	ticker := &kraken.TickerData{
		Symbol: "TEST11",
		Ask:    decimal.NewFromFloat64(125000.00),
		Bid:    decimal.NewFromFloat64(124950.00),
	}

	price.Update(ticker)

	b.ResetTimer()

	for b.Loop() {
		price.ExitValue(pair, holding)
	}
}

func BenchmarkPriceWithFriction(b *testing.B) {
	price := entryEconomicsFixture(b, 101, 100, 3)
	pair := kraken.InstrumentPair{Symbol: "EDGE/USD"}
	holding := &types.Holding{Qty: decimal.NewFromFloat64(2)}
	value := decimal.NewFromFloat64(10)
	b.ReportAllocs()

	for b.Loop() {
		// WithFriction always errors (no full-depth book is ever available);
		// this benchmark measures the cost of that fast-reject path.
		if _, err := price.WithFriction(pair, holding, value); err == nil {
			b.Fatal("expected WithFriction to error without a full-depth book")
		}
	}
}

func BenchmarkPriceTick(b *testing.B) {
	price, _ := newPriceSurface(b, "TEST14")

	ticker := &kraken.TickerData{
		Symbol: "TEST14",
		Ask:    decimal.NewFromFloat64(140000.00),
		Bid:    decimal.NewFromFloat64(139950.00),
	}
	price.Update(ticker)

	b.ResetTimer()

	for b.Loop() {
		price.Tick("TEST14")
	}
}

func BenchmarkPriceQuantity(b *testing.B) {
	price := newQuantityPrice(b)
	notional, err := decimal.NewFromString("20.00")

	if err != nil {
		b.Fatalf("notional: %v", err)
	}

	b.ResetTimer()

	for b.Loop() {
		price.Quantity("BTC/USD", notional)
	}
}
