package broker

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"

	. "github.com/smartystreets/goconvey/convey"
)

type priceTestPrivate struct {
	schedule  kraken.FeeSchedule
	postCalls int
}

func (private *priceTestPrivate) Client() *spot.WebSocket {
	return nil
}

func (private *priceTestPrivate) On(string, func([]byte)) {}

func (private *priceTestPrivate) Write(json.Marshaler) error {
	return nil
}

func (private *priceTestPrivate) Get(string, json.Marshaler) ([]byte, error) {
	return nil, nil
}

func (private *priceTestPrivate) Post(
	path string, _ json.Marshaler,
) ([]byte, error) {
	private.postCalls++

	if path != websocket.TradeVolumeEndpoint {
		return nil, nil
	}

	return sonic.Marshal(private.schedule)
}

func (private *priceTestPrivate) Close() {}

var _ websocket.Conn = (*priceTestPrivate)(nil)

func TestPricePreflight(t *testing.T) {
	Convey("Given a fresh executable quote within configured risk limits", t, func() {
		previousAge := viper.GetDuration("trading.max_quote_age")
		previousSpread := viper.GetFloat64("trading.max_spread_bps")
		defer viper.Set("trading.max_quote_age", previousAge)
		defer viper.Set("trading.max_spread_bps", previousSpread)
		viper.Set("trading.max_quote_age", time.Second)
		viper.Set("trading.max_spread_bps", 20)

		price := &Price{}
		price.tickers.Store(map[string]kraken.TickerData{
			"BTC/USD": {
				Bid:       *decimal.NewFromFloat64(100),
				Ask:       *decimal.NewFromFloat64(100.1),
				AskQty:    2,
				Timestamp: time.Now(),
			},
		})

		Convey("It accepts covered size and rejects uncovered size", func() {
			So(price.Preflight("BTC/USD", 1), ShouldBeNil)
			So(price.Preflight("BTC/USD", 3), ShouldNotBeNil)
		})
	})
}

const instrumentFrame = `{
	"channel": "instrument",
	"data": {
		"pairs": [{
			"symbol": "MANA/USD",
			"quote": "USD",
			"status": "online"
		}]
	}
}`

func TestPriceSymbol(t *testing.T) {
	Convey("Given Price with instrument and ticker handlers", t, func() {
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		defer viper.Set("market.quote_currency", previousQuote)

		private := &priceTestPrivate{}
		price := NewPrice(private, private)
		fees := NewFees(price)
		quote := NewQuote(price)

		Convey("When instrument and ticker rows arrive", func() {
			fees.On([]byte(instrumentFrame))
			quote.On([]byte(`[{
				"symbol": "MANA/USD",
				"bid": 0.066,
				"ask": 0.068,
				"last": 0.067
			}]`))

			Convey("Then Symbol returns the latest raw ticker price", func() {
				symbolPrice := price.Symbol("MANA/USD")
				So(symbolPrice.String(), ShouldEqual, "0.067")
			})
		})
	})
}

func TestPriceEntry(t *testing.T) {
	Convey("Given Price with instrument and ticker handlers", t, func() {
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		defer viper.Set("market.quote_currency", previousQuote)

		private := &priceTestPrivate{}
		price := NewPrice(private, private)
		fees := NewFees(price)
		quote := NewQuote(price)

		Convey("When instrument and ticker rows arrive", func() {
			fees.On([]byte(instrumentFrame))
			quote.On([]byte(`[{
				"symbol": "MANA/USD",
				"bid": 0.066,
				"ask": 0.068,
				"last": 0.067
			}]`))

			Convey("Then Entry returns the executable ask price", func() {
				entryPrice, ok := price.Entry("MANA/USD")
				So(ok, ShouldBeTrue)
				So(entryPrice.String(), ShouldEqual, "0.068")
			})
		})
	})
}

func TestPricePnL(t *testing.T) {
	Convey("Given Price with real TradeVolume fees and a live bid", t, func() {
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		defer viper.Set("market.quote_currency", previousQuote)

		private := &priceTestPrivate{}
		price := &Price{}
		price.tickers.Store(map[string]kraken.TickerData{
			"MANA/USD": {
				Symbol: "MANA/USD",
				Bid:    *decimal.NewFromFloat64(101),
				Ask:    *decimal.NewFromFloat64(102),
				Last:   *decimal.NewFromFloat64(101.5),
			},
		})
		price.fees.Store(map[string]kraken.FeeRates{
			"MANA/USD": {Taker: 0.001},
		})

		position := NewPosition(private, &PositionData{
			Symbol:     "MANA/USD",
			Qty:        1,
			EntryPrice: *decimal.NewFromFloat64(100),
		})

		Convey("Then PnL subtracts entry and exit fees using the executable bid", func() {
			ticker, ok := price.ticker("MANA/USD")
			So(ok, ShouldBeTrue)
			So(ticker.Bid.String(), ShouldEqual, "101")

			feeRate, ok := price.fee("MANA/USD")
			So(ok, ShouldBeTrue)
			So(feeRate, ShouldAlmostEqual, 0.001)

			pnl := price.PnL(position)
			So(pnl.String(), ShouldEqual, "0.799")
		})
	})
}

func TestPriceRoundTripFriction(t *testing.T) {
	Convey("Given Price with real TradeVolume fees and a live spread", t, func() {
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		defer viper.Set("market.quote_currency", previousQuote)

		private := &priceTestPrivate{
			schedule: kraken.FeeSchedule{
				Pairs: map[string]kraken.FeeRates{
					"MANA/USD": {Taker: 0.001},
				},
			},
		}
		price := NewPrice(private, private)
		fees := NewFees(price)
		quote := NewQuote(price)

		fees.On([]byte(instrumentFrame))
		quote.On([]byte(`[{
			"symbol": "MANA/USD",
			"bid": 100,
			"ask": 101,
			"last": 100.5
		}]`))

		Convey("Then RoundTripFriction prices spread and entry-exit fees", func() {
			friction, ok := price.RoundTripFriction("MANA/USD")
			So(ok, ShouldBeTrue)

			// Exact spread (1/100.5) plus 2x0.001 taker fee is 1201/100500,
			// rounded to the fee rate's 3-decimal scale since bid/ask carry
			// no fractional digits here.
			So(friction.String(), ShouldEqual, big.NewRat(1201, 100500).FloatString(3))
		})
	})
}

func TestQuoteOn(t *testing.T) {
	Convey("Given Price with an instrument-bounded symbol set", t, func() {
		price := &Price{}
		quote := NewQuote(price)
		price.symbols.Store(map[string]struct{}{
			"MANA/USD": {},
		})
		price.tickers.Store(map[string]kraken.TickerData{})
		price.fees.Store(map[string]kraken.FeeRates{})

		Convey("When ticker rows include symbols outside that set", func() {
			quote.On([]byte(`[{
				"symbol": "MANA/USD",
				"bid": 0.066,
				"ask": 0.068,
				"last": 0.067
			}, {
				"symbol": "DOGE/EUR",
				"bid": 0.1,
				"ask": 0.2,
				"last": 0.15
			}]`))

			Convey("Then only instrument-scoped tickers are retained", func() {
				_, ok := price.ticker("MANA/USD")
				So(ok, ShouldBeTrue)

				_, ok = price.ticker("DOGE/EUR")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestFeesSchedule(t *testing.T) {
	Convey("Given Price with an instrument-bounded symbol set", t, func() {
		price := &Price{}
		fees := NewFees(price)
		price.symbols.Store(map[string]struct{}{
			"MANA/USD": {},
		})
		price.tickers.Store(map[string]kraken.TickerData{})
		price.fees.Store(map[string]kraken.FeeRates{
			"OLD/USD": {Taker: 0.01},
		})

		Convey("When a fee schedule includes old and out-of-scope symbols", func() {
			fees.schedule(kraken.FeeSchedule{
				Pairs: map[string]kraken.FeeRates{
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

func TestFeesOn(t *testing.T) {
	Convey("Given Price with stale ticker and fee state", t, func() {
		private := &priceTestPrivate{}
		price := &Price{private: private}
		fees := NewFees(price)
		price.symbols.Store(map[string]struct{}{
			"OLD/USD": {},
		})
		price.tickers.Store(map[string]kraken.TickerData{
			"OLD/USD": {
				Symbol: "OLD/USD",
				Bid:    tests.Decimal(t, "1"),
				Ask:    tests.Decimal(t, "1.1"),
				Last:   tests.Decimal(t, "1"),
			},
		})
		price.fees.Store(map[string]kraken.FeeRates{
			"OLD/USD": {Taker: 0.01},
		})

		Convey("When an instrument snapshot has no tracked symbols", func() {
			previousQuote := viper.GetString("market.quote_currency")
			viper.Set("market.quote_currency", "USD")
			defer viper.Set("market.quote_currency", previousQuote)

			fees.On([]byte(`{
				"channel": "instrument",
				"data": {
					"pairs": [{
						"symbol": "DOGE/EUR",
						"quote": "EUR",
						"status": "online"
					}]
				}
			}`))

			Convey("Then stale ticker and fee state is cleared", func() {
				_, ok := price.ticker("OLD/USD")
				So(ok, ShouldBeFalse)

				_, ok = price.fee("OLD/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestFeesOnPost(t *testing.T) {
	Convey("Given a private transport that serves TradeVolume", t, func() {
		private := &priceTestPrivate{
			schedule: kraken.FeeSchedule{
				Pairs: map[string]kraken.FeeRates{
					"BTC/USD": {Taker: 0.0026, Maker: 0.0016},
				},
			},
		}
		price := &Price{private: private}
		fees := NewFees(price)

		Convey("When instrument data arrives", func() {
			previousQuote := viper.GetString("market.quote_currency")
			viper.Set("market.quote_currency", "USD")
			defer viper.Set("market.quote_currency", previousQuote)

			fees.On([]byte(`{
				"channel": "instrument",
				"data": {
					"pairs": [{
						"symbol": "BTC/USD",
						"quote": "USD",
						"status": "online"
					}]
				}
			}`))

			Convey("Then fees are loaded through the transport Post", func() {
				So(private.postCalls, ShouldEqual, 1)

				takerRate, ok := price.fee("BTC/USD")
				So(ok, ShouldBeTrue)
				So(takerRate, ShouldAlmostEqual, 0.0026, 1e-12)
			})
		})
	})
}

func BenchmarkPricePnL(b *testing.B) {
	price := &Price{}
	price.tickers.Store(map[string]kraken.TickerData{
		"MANA/USD": {
			Symbol: "MANA/USD",
			Bid:    *decimal.NewFromFloat64(101),
			Ask:    *decimal.NewFromFloat64(102),
			Last:   *decimal.NewFromFloat64(101.5),
		},
	})
	price.fees.Store(map[string]kraken.FeeRates{
		"MANA/USD": {Taker: 0.001},
	})
	position := NewPosition(&priceTestPrivate{}, &PositionData{
		Symbol:     "MANA/USD",
		Qty:        1,
		EntryPrice: *decimal.NewFromFloat64(100),
	})

	b.ReportAllocs()
	for b.Loop() {
		_ = price.PnL(position)
	}
}

func BenchmarkQuoteOn(b *testing.B) {
	price := &Price{}
	quote := NewQuote(price)
	price.symbols.Store(map[string]struct{}{
		"MANA/USD": {},
	})
	payload := []byte(`[{
		"symbol": "MANA/USD",
		"bid": 0.066,
		"ask": 0.068,
		"last": 0.067
	}]`)

	b.ReportAllocs()
	for b.Loop() {
		quote.On(payload)
	}
}
