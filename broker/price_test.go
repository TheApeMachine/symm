package broker

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func TestPriceTickerAck(t *testing.T) {
	Convey("Given a ticker envelope", t, func() {
		price := &Price{
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
		price.status = types.READY

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
	Convey("Given ticker rows for part of an expected identity set", t, func() {
		price := &Price{
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
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
	Convey("Given normalized fee tiers for the requested symbols and an unrelated symbol", t, func() {
		mock := tests.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFees{
				"BTC/USD": {Fee: "0.2600"},
				"ETH/USD": {Fee: "0.2600"},
				"OLD/USD": {Fee: "0.4000"},
			}},
		}), ShouldBeNil)
		price := NewPrice(websocket.NewAPI(context.Background(), mock.Public(), mock.Private(), nil))

		Convey("When the exact trading tier is hydrated", func() {
			err := price.GetFees([]string{"BTC/USD", "ETH/USD"})

			Convey("Then only that complete tier is committed before Price becomes ready", func() {
				So(err, ShouldBeNil)
				So(mock.LastTradeVolumeSymbols(), ShouldResemble, []string{"BTC/USD", "ETH/USD"})
				So(price.Status(), ShouldEqual, types.READY)

				btcFee, err := price.FeeRate("BTC/USD")
				So(err, ShouldBeNil)
				So(btcFee.Fee, ShouldEqual, "0.2600")

				_, err = price.FeeRate("OLD/USD")
				So(err, ShouldNotBeNil)
			})
		})
	})

	Convey("Given a fee response missing one requested symbol", t, func() {
		mock := tests.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFees{
				"BTC/USD": {Fee: "0.2600"},
			}},
		}), ShouldBeNil)
		price := NewPrice(websocket.NewAPI(context.Background(), mock.Public(), mock.Private(), nil))

		Convey("When fee hydration is attempted", func() {
			err := price.GetFees([]string{"BTC/USD", "ETH/USD"})

			Convey("Then Price becomes ready and only returned tiers are cached", func() {
				So(err, ShouldBeNil)
				So(price.Status(), ShouldEqual, types.READY)

				btcFee, err := price.FeeRate("BTC/USD")
				So(err, ShouldBeNil)
				So(btcFee.Fee, ShouldEqual, "0.2600")

				ethFee, err := price.FeeRate("ETH/USD")
				So(err, ShouldBeNil)
				So(ethFee.Fee, ShouldBeEmpty)
			})
		})
	})

	Convey("Given a malformed fee for one requested symbol", t, func() {
		mock := tests.NewMockAPI()
		So(mock.SetTradeVolumeResponse(&kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFees{
				"BTC/USD": {Fee: "invalid"},
			}},
		}), ShouldBeNil)
		price := NewPrice(websocket.NewAPI(context.Background(), mock.Public(), mock.Private(), nil))

		Convey("When fee hydration is attempted", func() {
			err := price.GetFees([]string{"BTC/USD"})

			Convey("Then the tier is cached and Price becomes ready", func() {
				So(err, ShouldBeNil)
				So(price.Status(), ShouldEqual, types.READY)

				btcFee, err := price.FeeRate("BTC/USD")
				So(err, ShouldBeNil)
				So(btcFee.Fee, ShouldEqual, "invalid")
			})
		})
	})
}

func TestPriceWithFriction(t *testing.T) {
	Convey("Given a price stream with a known taker fee", t, func() {
		price := &Price{
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
		price.status = types.READY
		price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
		last, err := decimal.NewFromString("50000.5")

		So(err, ShouldBeNil)

		price.tickers.Store("BTC/USD", &kraken.TickerData{
			Symbol: "BTC/USD",
			Last:   last,
		})

		Convey("When WithFriction is requested for unit quantity", func() {
			net, err := price.WithFriction(
				"BTC/USD", *decimal.NewFromInt64(1),
			)

			Convey("Then it returns the all-in round-trip taker quote", func() {
				// 50000.5 notional + two 0.26% taker fees:
				// fee = 50000.5 * 0.0026 = 130.0013, total = 260.0026.
				So(err, ShouldBeNil)
				So(net.Float64(), ShouldAlmostEqual, 50260.5026, 1e-8)
			})
		})
	})
}

func BenchmarkPriceWithFriction(b *testing.B) {
	price := &Price{
		fees:    &sync.Map{},
		tickers: &sync.Map{},
	}
	price.status = types.READY
	price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
	last, err := decimal.NewFromString("50000.5")

	if err != nil {
		b.Fatal(err)
	}

	price.tickers.Store("BTC/USD", &kraken.TickerData{
		Symbol: "BTC/USD",
		Last:   last,
	})

	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.WithFriction(
			"BTC/USD", *decimal.NewFromInt64(1),
		)
	}
}

func BenchmarkPriceSnapshot(b *testing.B) {
	price := &Price{
		fees:    &sync.Map{},
		tickers: &sync.Map{},
	}
	symbols := make([]string, 641)

	for index := range symbols {
		symbols[index] = fmt.Sprintf("ASSET-%03d/USD", index)
		price.tickers.Store(symbols[index], &kraken.TickerData{Symbol: symbols[index]})
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
	price := &Price{
		fees:    &sync.Map{},
		tickers: &sync.Map{},
	}
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
