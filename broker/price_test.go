package broker_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

func TestPriceTickerAck(t *testing.T) {
	Convey("Given a ticker envelope", t, func() {
		price := broker.NewPrice(nil)
		So(price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)

		price.TickerAck([]byte(`{
			"channel":"ticker",
			"type":"update",
			"data":[{"symbol":"BTC/USD","last":"42000.0","bid":"41999.0","ask":"42001.0"}]
		}`))

		Convey("It should cache ticker rows as pointers", func() {
			ticker, err := price.Get("BTC/USD")

			So(err, ShouldBeNil)
			So(ticker, ShouldNotBeNil)
			So(ticker.Symbol, ShouldEqual, "BTC/USD")
			So(ticker.Last.Float64(), ShouldEqual, 42000.0)
		})
	})

}

func TestPriceSnapshot(t *testing.T) {
	Convey("Given an initializing Price", t, func() {
		mock := mockapi.NewMockAPI()
		price := broker.NewPrice(websocket.NewAPI(
			context.Background(), mock.Public(), mock.Private(), nil,
		))

		Convey("When its ticker callback is registered", func() {
			err := price.Initialize()

			Convey("Then Price is ready independently of trading-tier fees", func() {
				So(err, ShouldBeNil)
				So(price.Status(), ShouldEqual, types.READY)
				_, feeErr := price.FeeRate("BTC/USD")
				So(feeErr, ShouldNotBeNil)
			})
		})
	})

	Convey("Given ticker rows for part of an expected identity set", t, func() {
		price := broker.NewPrice(nil)
		price.TickerAck([]byte(`{
			"channel":"ticker",
			"type":"snapshot",
			"data":[
				{"symbol":"BTC/USD","last":"42000","bid":"41999","ask":"42001"},
				{"symbol":"SOL/USD","last":"150","bid":"149","ask":"151"}
			]
		}`))

		Convey("When the exact expected symbols are read", func() {
			rows, missing := price.Snapshot([]string{"SOL/USD", "ETH/USD", "BTC/USD"})

			Convey("Then rows preserve requested identity order and missing identities are explicit", func() {
				So(rows, ShouldHaveLength, 2)
				So(rows[0].Symbol, ShouldEqual, "SOL/USD")
				So(rows[1].Symbol, ShouldEqual, "BTC/USD")
				So(missing, ShouldResemble, []string{"ETH/USD"})
			})
		})
	})
}

func TestPriceGetFees(t *testing.T) {
	Convey("Given a Kraken-keyed fee tier for one requested symbol", t, func() {
		mock := mockapi.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFee{
				"XXBTZUSD": {Fee: decimal.NewFromFloat64(0.26)},
			}},
		}), ShouldBeNil)
		api := websocket.NewAPI(context.Background(), mock.Public(), mock.Private(), nil)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)

		Convey("When the exact trading tier is hydrated", func() {
			err := price.GetFees([]string{"BTC/USD"})

			Convey("Then only that complete tier is committed before Price becomes ready", func() {
				So(err, ShouldBeNil)
				symbols, symbolsErr := mock.LastTradeVolumeSymbols()
				So(symbolsErr, ShouldBeNil)
				So(symbols, ShouldResemble, []string{"BTC/USD"})
				So(price.Status(), ShouldEqual, types.READY)

				btcFee, err := price.FeeRate("BTC/USD")
				So(err, ShouldBeNil)
				So(btcFee.Fee.Float64(), ShouldEqual, 0.26)
			})
		})
	})

	Convey("Given a fee response missing one requested symbol", t, func() {
		mock := mockapi.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFee{}},
		}), ShouldBeNil)
		api := websocket.NewAPI(context.Background(), mock.Public(), mock.Private(), nil)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)

		Convey("When fee hydration is attempted", func() {
			err := price.GetFees([]string{"BTC/USD"})

			Convey("Then the incomplete tier is rejected without becoming ready", func() {
				So(err, ShouldNotBeNil)
				So(price.Status(), ShouldNotEqual, types.READY)

				_, err = price.FeeRate("BTC/USD")
				So(err, ShouldNotBeNil)

			})
		})
	})

	Convey("Given a malformed fee for one requested symbol", t, func() {
		mock := mockapi.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFee{
				"XXBTZUSD": {},
			}},
		}), ShouldBeNil)
		api := websocket.NewAPI(context.Background(), mock.Public(), mock.Private(), nil)
		So(api.Initialize(), ShouldBeNil)
		price := broker.NewPrice(api)

		Convey("When fee hydration is attempted", func() {
			err := price.GetFees([]string{"BTC/USD"})

			Convey("Then the malformed tier is rejected without becoming ready", func() {
				So(err, ShouldNotBeNil)
				So(price.Status(), ShouldNotEqual, types.READY)

				_, err = price.FeeRate("BTC/USD")
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestPriceWithFriction(t *testing.T) {
	Convey("Given a price stream with a known taker fee", t, func() {
		price := broker.NewPrice(nil)
		So(price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)
		price.TickerAck([]byte(`{
			"channel":"ticker",
			"type":"update",
			"data":[{"symbol":"BTC/USD","last":"50000.5","bid":"50000.0","ask":"50000.5"}]
		}`))

		Convey("When WithFriction is requested for unit quantity", func() {
			net, err := price.WithFriction(
				&kraken.InstrumentPair{
					Symbol:         "BTC/USD",
					PricePrecision: 1,
					CostPrecision:  2,
				},
				decimal.NewFromInt64(1),
			)

			Convey("Then it returns the all-in round-trip taker quote", func() {
				// 50000.5 notional + two 0.26% taker fees:
				// fee = 50000.5 * 0.0026 = 130.0013, total = 260.0026.
				So(err, ShouldBeNil)
				So(net.Float64(), ShouldAlmostEqual, 50260.50, 1e-8)
			})
		})
	})
}

func BenchmarkPriceWithFriction(b *testing.B) {
	price := broker.NewPrice(nil)
	_ = price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price.TickerAck([]byte(`{
		"channel":"ticker",
		"type":"update",
		"data":[{"symbol":"BTC/USD","last":"50000.5","bid":"50000.0","ask":"50000.5"}]
	}`))

	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.WithFriction(
			&kraken.InstrumentPair{
				Symbol:         "BTC/USD",
				PricePrecision: 1,
				CostPrecision:  4,
			},
			decimal.NewFromInt64(1),
		)
	}
}

func BenchmarkPriceSnapshot(b *testing.B) {
	price := broker.NewPrice(nil)
	symbols := make([]string, 641)

	for index := range symbols {
		symbols[index] = fmt.Sprintf("ASSET-%03d/USD", index)
		price.TickerAck([]byte(fmt.Sprintf(
			`{"channel":"ticker","type":"update","data":[{"symbol":"%s","last":"1","bid":"1","ask":"1"}]}`,
			symbols[index],
		)))
	}

	b.ReportAllocs()

	for b.Loop() {
		rows, missing := price.Snapshot(symbols)

		if len(rows) != len(symbols) || len(missing) != 0 {
			b.Fatal("incomplete ticker snapshot")
		}
	}
}

func BenchmarkPriceTickerAck(b *testing.B) {
	price := broker.NewPrice(nil)
	frame := []byte(`{
		"channel":"ticker",
		"type":"update",
		"data":[{"symbol":"BTC/USD","last":"102","bid":"101","ask":"103"}]
	}`)

	b.ReportAllocs()

	for b.Loop() {
		price.TickerAck(frame)
	}
}
