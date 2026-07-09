package broker

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"

	. "github.com/smartystreets/goconvey/convey"
)

type priceTestPrivate struct {
	schedule websocket.FeeSchedule
}

func (private *priceTestPrivate) Observe(_ string) chan []byte {
	return make(chan []byte, 8)
}

func (private *priceTestPrivate) Submit(_ *kraken.Order) error {
	return nil
}

func (private *priceTestPrivate) TradeVolume(_ []string) (websocket.FeeSchedule, error) {
	return private.schedule, nil
}

func (private *priceTestPrivate) Close() {
}

func TestPriceSymbol(testingTB *testing.T) {
	Convey("Given Price observing the public ticker stream", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		public := &recordingSocket{}
		private := &priceTestPrivate{}
		price := NewPrice(ctx, public, private)
		defer price.Close()

		Convey("When a ticker row arrives", func() {
			public.channels["instrument"] <- []byte(`{
				"channel": "instrument",
				"data": {
					"pairs": [{
						"symbol": "MANA/USD",
						"quote": "USD",
						"status": "online"
					}]
				}
			}`)
			waitForPrice(testingTB, func() bool {
				symbols, _ := price.symbols.Load().(map[string]struct{})
				_, ok := symbols["MANA/USD"]
				return ok
			})

			public.channels[channelTicker] <- []byte(`[{
				"symbol": "MANA/USD",
				"bid": 0.066,
				"ask": 0.068,
				"last": 0.067
			}]`)

			waitForPrice(testingTB, func() bool {
				symbolPrice := price.Symbol("MANA/USD")
				return symbolPrice.Rat().Sign() > 0
			})

			Convey("Then Symbol returns the latest raw ticker price", func() {
				symbolPrice := price.Symbol("MANA/USD")
				So(symbolPrice.String(), ShouldEqual, "0.067")
			})
		})
	})
}

func TestPriceEntry(testingTB *testing.T) {
	Convey("Given Price observing the public ticker stream", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		public := &recordingSocket{}
		private := &priceTestPrivate{}
		price := NewPrice(ctx, public, private)
		defer price.Close()

		Convey("When a ticker row arrives", func() {
			public.channels["instrument"] <- []byte(`{
				"channel": "instrument",
				"data": {
					"pairs": [{
						"symbol": "MANA/USD",
						"quote": "USD",
						"status": "online"
					}]
				}
			}`)
			waitForPrice(testingTB, func() bool {
				symbols, _ := price.symbols.Load().(map[string]struct{})
				_, ok := symbols["MANA/USD"]
				return ok
			})

			public.channels[channelTicker] <- []byte(`[{
				"symbol": "MANA/USD",
				"bid": 0.066,
				"ask": 0.068,
				"last": 0.067
			}]`)

			waitForPrice(testingTB, func() bool {
				entryPrice, ok := price.Entry("MANA/USD")
				return ok && entryPrice.Rat().Sign() > 0
			})

			Convey("Then Entry returns the executable ask price", func() {
				entryPrice, ok := price.Entry("MANA/USD")
				So(ok, ShouldBeTrue)
				So(entryPrice.String(), ShouldEqual, "0.068")
			})
		})
	})
}

func TestPricePnL(testingTB *testing.T) {
	Convey("Given Price with real TradeVolume fees and a live bid", testingTB, func() {
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		defer viper.Set("market.quote_currency", previousQuote)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		public := &recordingSocket{}
		private := &priceTestPrivate{
			schedule: websocket.FeeSchedule{
				Pairs: map[string]websocket.FeeRates{
					"MANA/USD": {Taker: 0.001},
				},
			},
		}
		price := NewPrice(ctx, public, private)
		defer price.Close()

		public.channels["instrument"] <- []byte(`{
			"channel": "instrument",
			"data": {
				"pairs": [{
					"symbol": "MANA/USD",
					"quote": "USD",
					"status": "online"
				}]
			}
		}`)
		waitForPrice(testingTB, func() bool {
			_, ok := price.fee("MANA/USD")
			return ok
		})
		public.channels[channelTicker] <- []byte(`[{
			"symbol": "MANA/USD",
			"bid": 101,
			"ask": 102,
			"last": 101.5
		}]`)
		waitForPrice(testingTB, func() bool {
			symbolPrice := price.Symbol("MANA/USD")
			return symbolPrice.Rat().Sign() > 0
		})

		position := NewPosition(private, &PositionData{
			Symbol:     "MANA/USD",
			Qty:        1,
			EntryPrice: testDecimal(testingTB, "100"),
		})

		Convey("Then PnL subtracts entry and exit fees using the executable bid", func() {
			pnl := price.PnL(position)
			So(pnl.String(), ShouldEqual, "0.799")
		})
	})
}

func TestPriceRoundTripFriction(testingTB *testing.T) {
	Convey("Given Price with real TradeVolume fees and a live spread", testingTB, func() {
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		defer viper.Set("market.quote_currency", previousQuote)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		public := &recordingSocket{}
		private := &priceTestPrivate{
			schedule: websocket.FeeSchedule{
				Pairs: map[string]websocket.FeeRates{
					"MANA/USD": {Taker: 0.001},
				},
			},
		}
		price := NewPrice(ctx, public, private)
		defer price.Close()

		public.channels["instrument"] <- []byte(`{
			"channel": "instrument",
			"data": {
				"pairs": [{
					"symbol": "MANA/USD",
					"quote": "USD",
					"status": "online"
				}]
			}
		}`)
		waitForPrice(testingTB, func() bool {
			_, ok := price.fee("MANA/USD")
			return ok
		})
		public.channels[channelTicker] <- []byte(`[{
			"symbol": "MANA/USD",
			"bid": 100,
			"ask": 101,
			"last": 100.5
		}]`)

		Convey("Then RoundTripFriction prices spread and entry-exit fees", func() {
			waitForPrice(testingTB, func() bool {
				friction, ok := price.RoundTripFriction("MANA/USD")
				return ok && friction.Sign() > 0
			})

			friction, ok := price.RoundTripFriction("MANA/USD")
			So(ok, ShouldBeTrue)
			So(friction.Cmp(big.NewRat(1201, 100500)), ShouldEqual, 0)
		})
	})
}

func TestPriceObserveTickers(testingTB *testing.T) {
	Convey("Given Price with an instrument-bounded symbol set", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		channel := make(chan []byte, 8)
		price := &Price{
			ctx: ctx,
		}
		price.symbols.Store(map[string]struct{}{
			"MANA/USD": {},
		})
		price.tickers.Store(map[string]kraken.TickerData{})
		price.fees.Store(map[string]websocket.FeeRates{})
		go price.observeTickers(channel)

		Convey("When ticker rows include symbols outside that set", func() {
			channel <- []byte(`[{
				"symbol": "MANA/USD",
				"bid": 0.066,
				"ask": 0.068,
				"last": 0.067
			}, {
				"symbol": "DOGE/EUR",
				"bid": 0.1,
				"ask": 0.2,
				"last": 0.15
			}]`)

			Convey("Then only instrument-scoped tickers are retained", func() {
				waitForPrice(testingTB, func() bool {
					_, ok := price.ticker("MANA/USD")
					return ok
				})

				_, ok := price.ticker("MANA/USD")
				So(ok, ShouldBeTrue)

				_, ok = price.ticker("DOGE/EUR")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPriceObserveFeeSchedule(testingTB *testing.T) {
	Convey("Given Price with an instrument-bounded symbol set", testingTB, func() {
		price := &Price{}
		price.symbols.Store(map[string]struct{}{
			"MANA/USD": {},
		})
		price.tickers.Store(map[string]kraken.TickerData{})
		price.fees.Store(map[string]websocket.FeeRates{
			"OLD/USD": {Taker: 0.01},
		})

		Convey("When a fee schedule includes old and out-of-scope symbols", func() {
			price.observeFeeSchedule(websocket.FeeSchedule{
				Pairs: map[string]websocket.FeeRates{
					"MANA/USD": {Taker: 0.001},
					"DOGE/EUR": {Taker: 0.002},
				},
			})

			Convey("Then only instrument-scoped fees are retained", func() {
				rate, ok := price.fee("MANA/USD")
				So(ok, ShouldBeTrue)
				So(rate, ShouldAlmostEqual, 0.001)

				_, ok = price.fee("DOGE/EUR")
				So(ok, ShouldBeFalse)

				_, ok = price.fee("OLD/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPriceObserveInstruments(testingTB *testing.T) {
	Convey("Given Price with stale ticker and fee state", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		channel := make(chan []byte, 8)
		price := &Price{
			ctx:     ctx,
			private: &priceTestPrivate{},
		}
		price.symbols.Store(map[string]struct{}{
			"OLD/USD": {},
		})
		price.tickers.Store(map[string]kraken.TickerData{
			"OLD/USD": {
				Symbol: "OLD/USD",
				Bid:    testDecimal(testingTB, "1"),
				Ask:    testDecimal(testingTB, "1.1"),
				Last:   testDecimal(testingTB, "1"),
			},
		})
		price.fees.Store(map[string]websocket.FeeRates{
			"OLD/USD": {Taker: 0.01},
		})
		go price.observeInstruments(channel)

		Convey("When an instrument snapshot has no tracked symbols", func() {
			previousQuote := viper.GetString("market.quote_currency")
			viper.Set("market.quote_currency", "USD")
			defer viper.Set("market.quote_currency", previousQuote)

			channel <- []byte(`{
				"channel": "instrument",
				"data": {
					"pairs": [{
						"symbol": "DOGE/EUR",
						"quote": "EUR",
						"status": "online"
					}]
				}
			}`)

			Convey("Then stale ticker and fee state is cleared", func() {
				waitForPrice(testingTB, func() bool {
					_, ok := price.ticker("OLD/USD")
					return !ok
				})

				_, ok := price.ticker("OLD/USD")
				So(ok, ShouldBeFalse)

				_, ok = price.fee("OLD/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkPricePnL(benchmarkTB *testing.B) {
	price := &Price{}
	price.tickers.Store(map[string]kraken.TickerData{
		"MANA/USD": {
			Symbol: "MANA/USD",
			Bid:    *decimal.NewFromFloat64(101),
			Ask:    *decimal.NewFromFloat64(102),
			Last:   *decimal.NewFromFloat64(101.5),
		},
	})
	price.fees.Store(map[string]websocket.FeeRates{
		"MANA/USD": {Taker: 0.001},
	})
	position := NewPosition(&priceTestPrivate{}, &PositionData{
		Symbol:     "MANA/USD",
		Qty:        1,
		EntryPrice: *decimal.NewFromFloat64(100),
	})

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = price.PnL(position)
	}
}

func waitForPrice(testingTB *testing.T, ready func() bool) {
	deadline := time.After(time.Second)

	for {
		if ready() {
			return
		}

		select {
		case <-deadline:
			testingTB.Fatal("price state did not update")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
