package broker

import (
	"fmt"
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestPriceTickerAck(t *testing.T) {
	Convey("Given a ticker envelope", t, func() {
		price := &Price{
			fees:    &sync.Map{},
			tickers: &sync.Map{},
		}
		price.ready.Store(true)

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
		api := &priceAPIStub{tradeVolume: &kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFees{
				"BTC/USD": {Fee: "0.2600"},
				"ETH/USD": {Fee: "0.2600"},
				"OLD/USD": {Fee: "0.4000"},
			}},
		}}
		price := NewPrice(api, nil)

		Convey("When the exact trading tier is hydrated", func() {
			err := price.GetFees([]string{"BTC/USD", "ETH/USD"})

			Convey("Then only that complete tier is committed before Price becomes ready", func() {
				So(err, ShouldBeNil)
				So(api.requested, ShouldResemble, []string{"BTC/USD", "ETH/USD"})
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
		api := &priceAPIStub{tradeVolume: &kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFees{
				"BTC/USD": {Fee: "0.2600"},
			}},
		}}
		price := NewPrice(api, nil)

		Convey("When fee hydration is attempted", func() {
			err := price.GetFees([]string{"BTC/USD", "ETH/USD"})

			Convey("Then readiness is withheld and the missing identity is reported", func() {
				So(err, ShouldNotBeNil)
				So(price.Status(), ShouldEqual, types.INITIALIZING)
			})
		})
	})

	Convey("Given a malformed fee for one requested symbol", t, func() {
		api := &priceAPIStub{tradeVolume: &kraken.TradeVolume{
			Result: kraken.TradeVolumeResult{Fees: map[string]kraken.TradeVolumeFees{
				"BTC/USD": {Fee: "invalid"},
			}},
		}}
		price := NewPrice(api, nil)

		Convey("When fee hydration is attempted", func() {
			err := price.GetFees([]string{"BTC/USD"})

			Convey("Then invalid fee data cannot make Price ready", func() {
				So(err, ShouldNotBeNil)
				So(price.Status(), ShouldEqual, types.INITIALIZING)
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
		price.ready.Store(true)
		price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
		last, err := decimal.NewFromString("1")

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
				So(err, ShouldBeNil)
				So(net.Float64(), ShouldAlmostEqual, 1.00520676, 1e-8)
			})
		})
	})
}

func BenchmarkPriceWithFriction(b *testing.B) {
	price := &Price{
		fees:    &sync.Map{},
		tickers: &sync.Map{},
	}
	price.ready.Store(true)
	price.fees.Store("BTC/USD", kraken.TradeVolumeFees{Fee: "0.2600"})
	last, err := decimal.NewFromString("1")

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

type priceAPIStub struct {
	tradeVolume *kraken.TradeVolume
	requested   []string
}

func (stub *priceAPIStub) On(_ string, _ func([]byte)) {}

func (stub *priceAPIStub) TradeVolume(symbols []string) (*kraken.TradeVolume, error) {
	stub.requested = append([]string(nil), symbols...)
	return stub.tradeVolume, nil
}
