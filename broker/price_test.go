package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/mock"
	"github.com/theapemachine/symm/types"
)

/*
newPriceSurface creates a price surface with the symbol's executable fee row.
*/
func newPriceSurface(t testing.TB, symbol string) (*Price, *websocket.API) {
	t.Helper()
	api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())
	price := newTestPrice(t, api)
	price.fees.Store(symbol, kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.25),
	})

	return price, api
}

/*
newTestPrice builds a Price from an api, wiring the shared workspace the
constructor reads the api from.
*/
func newTestPrice(t testing.TB, api *websocket.API) *Price {
	t.Helper()
	bus := runtime.NewWorkspace(t.Context())
	bus.Share("api", api, "")

	return NewPrice(t.Context(), bus)
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

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST3",
				Ask:    decimal.NewFromFloat64(50000.00),
				Bid:    decimal.NewFromFloat64(49950.00),
			}

			price.Update(ticker)

			Convey("When the PnL is calculated for a holding", func() {
				pnl := price.PnL(pair, holding)

				Convey("It should return the profit or loss based on the current market best bid price", func() {
					So(pnl.Float64(), ShouldAlmostEqual, 4825.125, 1e-12)
				})
			})
		})
	})

	Convey("Given a holding before its first ticker arrives", t, func() {
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
		bid := 4338.33
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
		}
		price.Update(&kraken.TickerData{
			Symbol: "PAXG/USD",
			Ask:    decimal.NewFromFloat64(4339.02),
			Bid:    decimal.NewFromFloat64(bid),
		})

		Convey("It should multiply price and quantity without rounding quantity to a cent", func() {
			pnl := price.PnL(pair, holding)
			expected := bid*quantity*(1-0.0025) -
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

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST4",
				Ask:    decimal.NewFromFloat64(65000.00),
				Bid:    decimal.NewFromFloat64(64950.00),
			}

			price.Update(ticker)

			Convey("When the exit value is calculated for a holding", func() {
				exitValue := price.ExitValue(pair, holding)

				Convey("It should return the exit value based on the current market best bid price", func() {
					So(exitValue.Float64(), ShouldAlmostEqual, 129575.25, 1e-12)
				})
			})
		})
	})

	Convey("Given no holding or ticker", t, func() {
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
	Convey("Given an exit spanning ordered bid levels", t, func() {
		managed := entryEconomicsBook(
			t,
			bookLevel{book.Bid, 100, 1},
			bookLevel{book.Bid, 99, 2},
		)
		price := entryEconomicsManagedFixture(t, managed, 101, 100, 3)
		pair := kraken.InstrumentPair{Symbol: "EDGE/USD"}
		holding := &types.Holding{Qty: decimal.NewFromFloat64(2)}
		value := decimal.NewFromFloat64(10)

		adjusted, err := price.WithFriction(pair, holding, value)

		Convey("It should walk best bid downward without mutating the holding", func() {
			So(err, ShouldBeNil)
			So(adjusted.Float64(), ShouldAlmostEqual, 9.0025, 1e-12)
			So(holding.Qty.Float64(), ShouldEqual, 2.0)
		})
	})

	Convey("Given no authoritative exit book", t, func() {
		price := entryEconomicsFixture(t, 101, 100, 3)
		holding := &types.Holding{Qty: decimal.NewFromFloat64(1)}

		Convey("It should return an explicit error instead of an unexplained nil value", func() {
			adjusted, err := price.WithFriction(
				kraken.InstrumentPair{Symbol: "EDGE/USD"},
				holding,
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
		}
		price.Update(&kraken.TickerData{
			Symbol: "BTC/USD",
			Ask:    decimal.NewFromFloat64(64951.1),
			Bid:    decimal.NewFromFloat64(bid),
		})

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

func TestPriceExecutableSurface(t *testing.T) {
	Convey("Given a one-level full-coverage bid book", t, func() {
		managed := entryEconomicsBook(
			t,
			bookLevel{book.Bid, 100, 100000},
			bookLevel{book.Ask, 101, 100000},
		)
		price := entryEconomicsManagedFixture(t, managed, 101, 100, 100000)

		surface := price.ExecutableSurface(
			"EDGE/USD",
			decimal.NewFromFloat64(100000),
			nil,
			time.Now(),
		)

		Convey("it produces the expected executable mark and full coverage", func() {
			So(surface.BookComplete, ShouldBeTrue)
			So(surface.FullyExecutable, ShouldBeTrue)
			So(surface.ExecutableQty.Float64(), ShouldAlmostEqual, 100000, 1e-9)
			So(surface.ExecutableVWAP.Float64(), ShouldAlmostEqual, 100*0.9975, 1e-9)
		})
	})

	Convey("Given a multi-level bid book", t, func() {
		managed := entryEconomicsBook(
			t,
			bookLevel{book.Bid, 100, 50000},
			bookLevel{book.Bid, 90, 50000},
			bookLevel{book.Ask, 120, 100000},
		)
		price := entryEconomicsManagedFixture(t, managed, 120, 100, 100000)

		Convey("it walks the levels and produces the correct executable VWAP", func() {
			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(100000),
				nil,
				time.Now(),
			)

			gross := 100.0*50000 + 90.0*50000
			expectedVWAP := gross * 0.9975 / 100000

			So(surface.FullyExecutable, ShouldBeTrue)
			So(surface.ExecutableVWAP.Float64(), ShouldAlmostEqual, expectedVWAP, 1e-9)
		})

		Convey("the executable mark uses the actual SellableQty", func() {
			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(50000),
				nil,
				time.Now(),
			)

			So(surface.FullyExecutable, ShouldBeTrue)
			So(surface.ExecutableVWAP.Float64(), ShouldAlmostEqual, 100*0.9975, 1e-9)
		})
	})

	Convey("Given a book that cannot fill the complete lot", t, func() {
		managed := entryEconomicsBook(
			t,
			bookLevel{book.Bid, 100, 24000},
			bookLevel{book.Ask, 101, 100000},
		)
		price := entryEconomicsManagedFixture(t, managed, 101, 100, 24000)

		Convey("insufficient visible quantity is explicitly incomplete, never ticker fallback", func() {
			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(100000),
				nil,
				time.Now(),
			)

			So(surface.BookComplete, ShouldBeTrue)
			So(surface.FullyExecutable, ShouldBeFalse)
			So(surface.ExecutableVWAP, ShouldBeNil)
		})
	})

	Convey("Given a protected floor", t, func() {
		managed := entryEconomicsBook(
			t,
			bookLevel{book.Bid, 110, 24000},
			bookLevel{book.Bid, 95, 100000},
			bookLevel{book.Ask, 120, 100000},
		)
		price := entryEconomicsManagedFixture(t, managed, 120, 110, 100000)

		Convey("floor coverage reflects only quantity at or above the floor", func() {
			surface := price.ExecutableSurface(
				"EDGE/USD",
				decimal.NewFromFloat64(100000),
				decimal.NewFromFloat64(100),
				time.Now(),
			)

			So(surface.FloorCoverageQty.Float64(), ShouldAlmostEqual, 24000, 1e-9)
			So(surface.ExecutableQty.Float64(), ShouldAlmostEqual, 124000, 1e-9)
		})
	})
}

func BenchmarkPriceExecutableSurface(b *testing.B) {
	managed := entryEconomicsBook(
		b,
		bookLevel{book.Bid, 100, 50000},
		bookLevel{book.Bid, 99, 50000},
		bookLevel{book.Bid, 50, 50000},
		bookLevel{book.Ask, 101, 100000},
	)
	price := entryEconomicsManagedFixture(b, managed, 101, 100, 100000)
	sellable := decimal.NewFromFloat64(100000)
	floor := decimal.NewFromFloat64(51)
	at := time.Now()
	b.ReportAllocs()

	for b.Loop() {
		price.ExecutableSurface("EDGE/USD", sellable, floor, at)
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
	managed := entryEconomicsBook(
		b,
		bookLevel{book.Bid, 100, 1},
		bookLevel{book.Bid, 99, 2},
	)
	price := entryEconomicsManagedFixture(b, managed, 101, 100, 3)
	pair := kraken.InstrumentPair{Symbol: "EDGE/USD"}
	holding := &types.Holding{Qty: decimal.NewFromFloat64(2)}
	value := decimal.NewFromFloat64(10)
	b.ReportAllocs()

	for b.Loop() {
		if _, err := price.WithFriction(pair, holding, value); err != nil {
			b.Fatal(err)
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
