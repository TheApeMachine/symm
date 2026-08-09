package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mock"
	"github.com/theapemachine/symm/types"
)

/*
newPriceSurface creates a price surface with the symbol's executable fee row.
*/
func newPriceSurface(testCase testing.TB, symbol string) (*Price, *websocket.API) {
	testCase.Helper()
	api := websocket.NewAPI(testCase.Context(), mock.NewConn(), mock.NewConn())
	price := NewPrice(api)
	price.fees.Store(symbol, kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.25),
	})

	return price, api
}

/*
newQuantityPrice creates the executable BTC/USD quantity fixture.
*/
func newQuantityPrice(testCase testing.TB) *Price {
	testCase.Helper()
	price, api := newPriceSurface(testCase, "BTC/USD")
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
		notional := decimal.ExactMul(decimal.NewFromInt64(100), fraction)

		Convey("When the allocated cash is converted at the fee-adjusted ask", func() {
			quantity := price.Quantity("BTC/USD", notional)

			Convey("Then receiver precision should not erase the order quantity", func() {
				So(quantity, ShouldNotBeNil)
				So(quantity.Sign(), ShouldEqual, 1)
				So(quantity.String(), ShouldEqual, "0.00019950")
			})
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
