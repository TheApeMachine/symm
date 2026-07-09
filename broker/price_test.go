package broker

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"

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
		price, err := NewPrice(ctx, public, private)
		So(err, ShouldBeNil)
		defer price.Close()

		Convey("When a ticker row arrives", func() {
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
		price, err := NewPrice(ctx, public, private)
		So(err, ShouldBeNil)
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
		public.channels[channelTicker] <- []byte(`[{
			"symbol": "MANA/USD",
			"bid": 101,
			"ask": 102,
			"last": 101.5
		}]`)

		waitForPrice(testingTB, func() bool {
			_, ok := price.fee("MANA/USD")
			return ok
		})
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

func TestPricePredicted(testingTB *testing.T) {
	Convey("Given Price with prediction rows from the resonance side", testingTB, func() {
		price := &Price{}
		price.tickers.Store(map[string]kraken.TickerData{})
		price.fees.Store(map[string]websocket.FeeRates{})
		price.predictions.Store(map[string][]types.Prediction{})

		price.ObservePredictions([]types.Prediction{
			{
				Symbol:    "MANA/USD",
				Timestamp: 10,
				Price:     testDecimal(testingTB, "0.067"),
			},
			{
				Symbol:    "MANA/USD",
				Timestamp: 11,
				Price:     testDecimal(testingTB, "0.069"),
			},
		})

		Convey("Then Predicted returns a defensive copy of that symbol window", func() {
			rows := price.Predicted("MANA/USD")
			So(rows, ShouldHaveLength, 2)
			So(rows[1].Price.String(), ShouldEqual, "0.069")

			rows[1].Price = testDecimal(testingTB, "0.001")
			So(price.Predicted("MANA/USD")[1].Price.String(), ShouldEqual, "0.069")
		})

		Convey("Then Predicted balances historical and future rows around now", func() {
			now := uint64(time.Now().UnixNano())
			price.ObservePredictions([]types.Prediction{
				{
					Symbol:    "MANA/USD",
					Timestamp: now - uint64(3*time.Second),
					Price:     testDecimal(testingTB, "0.063"),
				},
				{
					Symbol:    "MANA/USD",
					Timestamp: now - uint64(2*time.Second),
					Price:     testDecimal(testingTB, "0.064"),
				},
				{
					Symbol:    "MANA/USD",
					Timestamp: now - uint64(time.Second),
					Price:     testDecimal(testingTB, "0.065"),
				},
				{
					Symbol:    "MANA/USD",
					Timestamp: now + uint64(time.Second),
					Price:     testDecimal(testingTB, "0.066"),
				},
			})

			rows := price.Predicted("MANA/USD")
			So(rows, ShouldHaveLength, 2)
			So(rows[0].Timestamp, ShouldEqual, now-uint64(time.Second))
			So(rows[1].Timestamp, ShouldEqual, now+uint64(time.Second))
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
	price.predictions.Store(map[string][]types.Prediction{})
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
