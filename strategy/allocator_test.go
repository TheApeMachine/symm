package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestAllocatorFrictionUsesPriceFraction ensures Decide receives fee-ready
forecasts from Price rather than trader-side percent math.
*/
func TestAllocatorFrictionUsesPriceFraction(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(nil)

	if err := price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	}); err != nil {
		t.Fatalf("RememberFee: %v", err)
	}

	allocator := NewAllocator(context.Background(), nil, nil, price)
	thesis := types.NewThesis(nil, nil)
	thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
		Source: "resonance+causal", Symbol: "BTC/USD",
		At: time.Unix(1, 0).UTC(), ObservedInterval: time.Second,
		SourceEpoch: 1, HorizonEvents: 1, ExpiresEpoch: 2,
		Target: "next_l3_epoch_mid_log_return", ModelVersion: "v1",
		Ready: true, Calibrated: true, CalibrationSamples: 8,
		ExpectedReturn: 0.01, ReferencePrice: 100,
		BuyCapacity: 10, SellCapacity: 10, ExpectedSpread: 0.001,
		Uncertainty: 0.01, Confidence: 0.8,
	})

	fees := allocator.Friction(thesis)

	if !thesis.Forecasts[0].FrictionReady {
		t.Fatal("want FrictionReady after Price.Fraction")
	}

	if thesis.Forecasts[0].ExpectedFees != 0.0026 {
		t.Fatalf("want fee fraction 0.0026, got %v", thesis.Forecasts[0].ExpectedFees)
	}

	if fees["BTC/USD"] != 0.0026 {
		t.Fatalf("want fees map entry 0.0026, got %v", fees["BTC/USD"])
	}
}

/*
TestAllocatorSizesQuantityBeforeTaker floors at qty_min when the fraction
budget is small, then prices that lot through Taker.
*/
func TestAllocatorSizesQuantityBeforeTaker(t *testing.T) {
	t.Parallel()

	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })

	quote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() { viper.Set("market.quote_currency", quote) })

	balance := broker.NewBalance(nil, nil, nil)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"10000","available":"10000","reserved":"0"` +
			`}]}`,
	))

	instrument := broker.NewInstrument(nil, nil, nil)
	instrument.On([]byte(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5` +
			`}]}}`,
	))

	price := broker.NewPrice(nil)

	if err := price.RememberFee("LRC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	}); err != nil {
		t.Fatalf("RememberFee: %v", err)
	}

	price.TickerAck([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"` +
			`}]}`,
	))

	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis(nil, nil)
	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action: "enter", Symbol: "LRC/USD", Utility: 0.05,
	})
	thesis.Holdings.Store("LRC/USD", types.Holding{
		Symbol: "LRC/USD",
		Qty:    decimal.NewFromFloat64(3),
		Order: &spot.Order{
			Description: &spot.OrderDescription{
				Pair: "LRC/USD", Type: "enter", OrderType: "market",
			},
			Volume: decimal.NewFromFloat64(3),
		},
	})

	allocator.Allocate(thesis)

	if allocator.Action != "enter" {
		t.Fatalf("want enter sizing, got %s (%s)", allocator.Action, allocator.Reason)
	}

	if allocator.Quantity == nil || allocator.Quantity.Float64() < 50 {
		t.Fatalf("want qty >= qty_min 50, got %v", allocator.Quantity)
	}

	if allocator.Cost == nil || allocator.Cost.Sign() <= 0 {
		t.Fatalf("want positive Taker cost, got %v", allocator.Cost)
	}

	if thesis.Decisions[0].ProposedQuantity < 50 {
		t.Fatalf(
			"decision qty must reflect sized lot, got %v",
			thesis.Decisions[0].ProposedQuantity,
		)
	}
}

/*
BenchmarkAllocatorAllocate measures one enter sizing pass.
*/
func BenchmarkAllocatorAllocate(b *testing.B) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	b.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })

	quote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	b.Cleanup(func() { viper.Set("market.quote_currency", quote) })

	balance := broker.NewBalance(nil, nil, nil)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"10000","available":"10000","reserved":"0"` +
			`}]}`,
	))

	instrument := broker.NewInstrument(nil, nil, nil)
	instrument.On([]byte(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5` +
			`}]}}`,
	))

	price := broker.NewPrice(nil)
	_ = price.RememberFee("LRC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price.TickerAck([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"` +
			`}]}`,
	))

	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis(nil, nil)
	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action: "enter", Symbol: "LRC/USD",
	})

	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		thesis.Holdings.Store("LRC/USD", types.Holding{
			Symbol: "LRC/USD",
			Qty:    decimal.NewFromFloat64(3),
			Order: &spot.Order{
				Description: &spot.OrderDescription{
					Pair: "LRC/USD", Type: "enter", OrderType: "market",
				},
				Volume: decimal.NewFromFloat64(3),
			},
		})
		allocator.Allocate(thesis)
	}
}
