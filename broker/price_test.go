package broker_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mock"
	"github.com/theapemachine/symm/types"
)

func TestPriceUpdate(t *testing.T) {
	Convey("Setup", t, func() {
		api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())
		price := broker.NewPrice(api)

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
		api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())
		price := broker.NewPrice(api)

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST2",
				Ask:    decimal.NewFromFloat64(40000.00),
				Bid:    decimal.NewFromFloat64(39950.00),
			}

			price.Update(ticker)

			Convey("When the mark price is requested for buying", func() {
				markPrice := price.Mark("TEST2", broker.BUY)

				Convey("It should return the average of the bid and ask prices", func() {
					expectedMarkPrice := ticker.Ask.Add(ticker.Bid).Div(decimal.NewFromFloat64(2))
					So(markPrice, ShouldEqual, expectedMarkPrice)
				})
			})

			Convey("When the mark price is requested for selling", func() {
				markPrice := price.Mark("TEST2", broker.SELL)

				Convey("It should return the average of the bid and ask prices", func() {
					expectedMarkPrice := ticker.Ask.Add(ticker.Bid).Div(decimal.NewFromFloat64(2))
					So(markPrice, ShouldEqual, expectedMarkPrice)
				})
			})
		})
	})
}

func TestPricePnL(t *testing.T) {
	Convey("Setup", t, func() {
		api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())
		price := broker.NewPrice(api)
		pair := kraken.InstrumentPair{Base: "BTC", Quote: "USD"}
		holding := &types.Holding{
			Symbol:     "TEST3",
			Qty:        decimal.NewFromFloat64(1.0),
			EntryPrice: decimal.NewFromFloat64(45000.00),
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
					expectedPnL := ticker.Bid.Sub(decimal.NewFromFloat64(1000.00))
					So(pnl, ShouldEqual, expectedPnL)
				})
			})
		})
	})
}

func TestExitValue(t *testing.T) {
	Convey("Setup", t, func() {
		api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())
		price := broker.NewPrice(api)
		pair := kraken.InstrumentPair{Base: "BTC", Quote: "USD"}
		holding := &types.Holding{
			Symbol:     "TEST4",
			Qty:        decimal.NewFromFloat64(2.0),
			EntryPrice: decimal.NewFromFloat64(60000.00),
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
					expectedExitValue := ticker.Bid.Mul(holding.Qty)
					So(exitValue, ShouldEqual, expectedExitValue)
				})
			})
		})
	})
}

func TestPriceWithFee(t *testing.T) {
	Convey("Setup", t, func() {
		api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())
		price := broker.NewPrice(api)

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST5",
				Ask:    decimal.NewFromFloat64(70000.00),
				Bid:    decimal.NewFromFloat64(69950.00),
			}

			price.Update(ticker)

			Convey("When the price with fee is calculated for buying", func() {
				priceWithFee := price.WithFee("TEST5", ticker.Ask, broker.BUY)

				Convey("It should return the price with the taker fee applied", func() {
					expectedPriceWithFee := ticker.Ask.Mul(decimal.NewFromFloat64(1.0025)) // Assuming a 0.25% taker fee
					So(priceWithFee, ShouldEqual, expectedPriceWithFee)
				})
			})

			Convey("When the price with fee is calculated for selling", func() {
				priceWithFee := price.WithFee("TEST5", ticker.Bid, broker.SELL)

				Convey("It should return the price with the taker fee applied", func() {
					expectedPriceWithFee := ticker.Bid.Mul(decimal.NewFromFloat64(0.9975)) // Assuming a 0.25% taker fee
					So(priceWithFee, ShouldEqual, expectedPriceWithFee)
				})
			})
		})
	})
}

func TestPriceFee(t *testing.T) {
	Convey("Setup", t, func() {
		api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())
		price := broker.NewPrice(api)

		Convey("When the fee is requested for a symbol", func() {
			fee := price.Fee("TEST6")

			Convey("It should return the taker fee for that symbol", func() {
				expectedFee := decimal.NewFromFloat64(0.0025) // Assuming a 0.25% taker fee
				So(fee.Fee, ShouldEqual, expectedFee)
			})
		})
	})
}

func TestPriceTick(t *testing.T) {
	Convey("Setup", t, func() {
		api := websocket.NewAPI(t.Context(), mock.NewConn(), mock.NewConn())
		price := broker.NewPrice(api)

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

func BenchmarkPriceUpdate(b *testing.B) {
	api := websocket.NewAPI(b.Context(), mock.NewConn(), mock.NewConn())
	price := broker.NewPrice(api)

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
	api := websocket.NewAPI(b.Context(), mock.NewConn(), mock.NewConn())
	price := broker.NewPrice(api)

	ticker := &kraken.TickerData{
		Symbol: "TEST9",
		Ask:    decimal.NewFromFloat64(100000.00),
		Bid:    decimal.NewFromFloat64(99950.00),
	}

	price.Update(ticker)

	b.ResetTimer()

	for b.Loop() {
		price.Mark("TEST9", broker.BUY)
	}
}

func BenchmarkPricePnL(b *testing.B) {
	api := websocket.NewAPI(b.Context(), mock.NewConn(), mock.NewConn())
	price := broker.NewPrice(api)
	pair := kraken.InstrumentPair{Base: "BTC", Quote: "USD"}
	holding := &types.Holding{
		Symbol:     "TEST10",
		Qty:        decimal.NewFromFloat64(1.0),
		EntryPrice: decimal.NewFromFloat64(110000.00),
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
	api := websocket.NewAPI(b.Context(), mock.NewConn(), mock.NewConn())
	price := broker.NewPrice(api)
	pair := kraken.InstrumentPair{Base: "BTC", Quote: "USD"}
	holding := &types.Holding{
		Symbol:     "TEST11",
		Qty:        decimal.NewFromFloat64(2.0),
		EntryPrice: decimal.NewFromFloat64(120000.00),
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

func BenchmarkPriceWithFee(b *testing.B) {
	api := websocket.NewAPI(b.Context(), mock.NewConn(), mock.NewConn())
	price := broker.NewPrice(api)

	ticker := &kraken.TickerData{
		Symbol: "TEST12",
		Ask:    decimal.NewFromFloat64(130000.00),
		Bid:    decimal.NewFromFloat64(129950.00),
	}

	price.Update(ticker)

	b.ResetTimer()

	for b.Loop() {
		price.WithFee("TEST12", ticker.Ask, broker.BUY)
	}
}

func BenchmarkPriceFee(b *testing.B) {
	api := websocket.NewAPI(b.Context(), mock.NewConn(), mock.NewConn())
	price := broker.NewPrice(api)

	b.ResetTimer()

	for b.Loop() {
		price.Fee("TEST13")
	}
}

func BenchmarkPriceTick(b *testing.B) {
	api := websocket.NewAPI(b.Context(), mock.NewConn(), mock.NewConn())
	price := broker.NewPrice(api)

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
