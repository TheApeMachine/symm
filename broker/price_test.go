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

/*
TestPrice_DivFloor proves liquidity-capped sell quantities never round above
the exact quote capacity that funds them.
*/
func TestPrice_DivFloor(t *testing.T) {
	Convey("Given quote capacity that does not divide evenly by its mark", t, func() {
		price := broker.NewPrice(nil)
		capacity := decimal.NewFromInt64(10)
		mark := decimal.NewFromInt64(3)

		Convey("It should floor the executable quantity at the requested scale", func() {
			quantity := price.DivFloor(capacity, mark, 3)
			So(quantity.String(), ShouldEqual, "3.333")
			So(price.Mul(mark, quantity).Cmp(capacity), ShouldBeLessThanOrEqualTo, 0)
		})
	})
}

/*
TestPrice_Notional proves executable quantity flooring cannot change the
rounding policy used by downstream quote-currency fee accounting.
*/
func TestPrice_Notional(t *testing.T) {
	Convey("Given equal quantities with banker's and executable floor rounding", t, func() {
		price := broker.NewPrice(nil)
		So(price.RememberFee("BILL/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.40),
		}), ShouldBeNil)
		pair := &kraken.InstrumentPair{
			Symbol:         "BILL/USD",
			PricePrecision: 5,
			CostPrecision:  5,
		}
		rate, err := decimal.NewFromString("0.02424")
		So(err, ShouldBeNil)
		bankerQuantity, err := decimal.NewFromString("100.00000")
		So(err, ShouldBeNil)
		flooredQuantity := price.DivFloor(
			decimal.NewFromInt64(300),
			decimal.NewFromInt64(3),
			5,
		)

		Convey("It should keep quantity rounding out of quote-currency fees", func() {
			bankerNotional := price.Notional(pair, rate, bankerQuantity)
			flooredNotional := price.Notional(pair, rate, flooredQuantity)
			bankerFee := price.Fee(pair, bankerNotional)
			flooredFee := price.Fee(pair, flooredNotional)

			So(flooredNotional.Cmp(bankerNotional), ShouldEqual, 0)
			So(flooredFee.Cmp(bankerFee), ShouldEqual, 0)
			So(flooredFee.String(), ShouldEqual, "0.00970")
		})

		Convey("It should round only after multiplying the complete operands", func() {
			preciseRate, parseErr := decimal.NewFromString("1.23456")
			So(parseErr, ShouldBeNil)
			preciseQuantity, parseErr := decimal.NewFromString("0.12345678")
			So(parseErr, ShouldBeNil)

			notional := price.Notional(pair, preciseRate, preciseQuantity)

			So(notional.String(), ShouldEqual, "0.15241")
		})
	})
}

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

func TestPriceFraction(t *testing.T) {
	Convey("Given a cached TradeVolume percent fee", t, func() {
		price := broker.NewPrice(nil)
		So(price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)

		Convey("When Fraction is requested", func() {
			fraction, err := price.Fraction("BTC/USD")

			Convey("Then percent is converted once on the Price surface", func() {
				So(err, ShouldBeNil)
				So(fraction.Float64(), ShouldEqual, 0.0026)
			})
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
	Convey("Given a live bid and paid entry friction", t, func() {
		price := broker.NewPrice(nil)
		So(price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		}), ShouldBeNil)
		price.TickerAck([]byte(`{
			"channel":"ticker",
			"type":"update",
			"data":[{"symbol":"BTC/USD","last":"50000.5","bid":"50000.0","ask":"50000.5"}]
		}`))
		pair := &kraken.InstrumentPair{
			Symbol:         "BTC/USD",
			PricePrecision: 1,
			CostPrecision:  2,
		}
		qty := decimal.NewFromInt64(1)
		entry := decimal.NewFromFloat64(50000.0)
		entryFee := price.Fee(pair, price.Notional(pair, entry, qty))

		Convey("When WithFriction scores flatten-now PnL", func() {
			pnl, err := price.WithFriction(pair, qty, entry, entryFee)

			Convey("Then PnL is (bid − exit fee) − (entry + entry fee)", func() {
				// bid 50000 − exit 130 − entry 50000 − entryFee 130 = −260
				So(err, ShouldBeNil)
				So(pnl.Float64(), ShouldAlmostEqual, -260.0, 1e-8)
			})
		})

		Convey("When Mark stamps a holding", func() {
			holding := &types.Holding{
				Symbol:     "BTC/USD",
				Qty:        qty,
				EntryPrice: entry,
				EntryFee:   entryFee,
			}
			err := price.Mark(pair, holding)

			Convey("Then bid, PnL, and return land without caller money math", func() {
				So(err, ShouldBeNil)
				So(holding.Mark.Float64(), ShouldAlmostEqual, 50000.0, 1e-8)
				So(holding.PnL.Float64(), ShouldAlmostEqual, -260.0, 1e-8)
				So(holding.ReturnPct, ShouldNotBeNil)
				So(*holding.ReturnPct, ShouldAlmostEqual, -260.0/50130.0, 1e-8)
			})
		})

		Convey("When Prorate scales remaining entry fee", func() {
			scaled := price.Prorate(
				entryFee,
				decimal.NewFromFloat64(0.5),
				decimal.NewFromInt64(1),
			)

			Convey("Then half the fee remains on the lot", func() {
				So(scaled.Float64(), ShouldAlmostEqual, entryFee.Float64()/2, 1e-8)
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
	pair := &kraken.InstrumentPair{
		Symbol:         "BTC/USD",
		PricePrecision: 1,
		CostPrecision:  4,
	}
	qty := decimal.NewFromInt64(1)
	entry := decimal.NewFromFloat64(50000.0)
	entryFee := price.Fee(pair, price.Notional(pair, entry, qty))

	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.WithFriction(pair, qty, entry, entryFee)
	}
}

func BenchmarkPriceSnapshot(b *testing.B) {
	price := broker.NewPrice(nil)
	symbols := make([]string, 641)

	for index := range symbols {
		symbols[index] = fmt.Sprintf("ASSET-%03d/USD", index)
		price.TickerAck(fmt.Appendf(nil,
			`{"channel":"ticker","type":"update","data":[{"symbol":"%s","last":"1","bid":"1","ask":"1"}]}`,
			symbols[index],
		))
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

func TestPriceQuantityBillFitsCashSlice(t *testing.T) {
	Convey("Given BILL/USD ask and a max_fraction cash slice", t, func() {
		price := broker.NewPrice(nil)
		So(price.RememberFee("BILL/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.40),
		}), ShouldBeNil)
		price.TickerAck([]byte(`{
			"channel":"ticker","type":"update",
			"data":[{"symbol":"BILL/USD","ask":"0.02424","bid":"0.02421","last":"0.02437"}]
		}`))

		pair := &kraken.InstrumentPair{
			Symbol:         "BILL/USD",
			QtyMin:         decimal.NewFromInt64(100),
			QtyIncrement:   decimal.NewFromFloat64(0.00001),
			QtyPrecision:   5,
			PricePrecision: 5,
			CostPrecision:  5,
			CostMin:        decimal.NewFromFloat64(0.5),
		}
		budget := decimal.NewFromFloat64(23.746)

		quantity, err := price.Quantity(pair, budget)

		Convey("Then Quantity fits inside the slice", func() {
			So(err, ShouldBeNil)
			So(quantity, ShouldNotBeNil)
			cost, costErr := price.Taker(pair, quantity)
			So(costErr, ShouldBeNil)
			So(cost.Cmp(budget) <= 0, ShouldBeTrue)
		})
	})
}

func TestPriceQuantityRejectsBudgetBelowMinimum(t *testing.T) {
	Convey("Given a budget that cannot fund QtyMin", t, func() {
		price := broker.NewPrice(nil)
		So(price.RememberFee("BILL/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.40),
		}), ShouldBeNil)
		price.TickerAck([]byte(`{
			"channel":"ticker","type":"update",
			"data":[{"symbol":"BILL/USD","ask":"0.02424","bid":"0.02421","last":"0.02437"}]
		}`))

		pair := &kraken.InstrumentPair{
			Symbol:         "BILL/USD",
			QtyMin:         decimal.NewFromInt64(100),
			QtyIncrement:   decimal.NewFromFloat64(0.00001),
			QtyPrecision:   5,
			PricePrecision: 5,
			CostPrecision:  5,
		}

		quantity, err := price.Quantity(pair, decimal.NewFromFloat64(0.1))

		Convey("Then Quantity fails before upsizing into an unaffordable lot", func() {
			So(quantity, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "below instrument minimum")
		})
	})

	Convey("Given an instrument without an exact qty_min", t, func() {
		price := broker.NewPrice(nil)
		So(price.RememberFee("BILL/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.40),
		}), ShouldBeNil)
		price.TickerAck([]byte(`{
			"channel":"ticker","type":"update",
			"data":[{"symbol":"BILL/USD","ask":"0.02424"}]
		}`))
		pair := &kraken.InstrumentPair{
			Symbol:        "BILL/USD",
			QtyPrecision:  5,
			CostPrecision: 5,
		}

		quantity, err := price.Quantity(pair, decimal.NewFromInt64(1))

		Convey("Then Quantity refuses to invent an executable minimum", func() {
			So(quantity, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "qty_min unavailable")
		})
	})
}

func BenchmarkPriceQuantity(b *testing.B) {
	price := broker.NewPrice(nil)
	_ = price.RememberFee("BILL/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.40),
	})
	price.TickerAck([]byte(`{
		"channel":"ticker","type":"update",
		"data":[{"symbol":"BILL/USD","ask":"0.02424","bid":"0.02421","last":"0.02437"}]
	}`))
	pair := &kraken.InstrumentPair{
		Symbol:         "BILL/USD",
		QtyMin:         decimal.NewFromInt64(100),
		QtyIncrement:   decimal.NewFromFloat64(0.00001),
		QtyPrecision:   5,
		PricePrecision: 5,
		CostPrecision:  5,
	}
	budget := decimal.NewFromFloat64(23.746)

	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.Quantity(pair, budget)
	}
}
