package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestAllocatorSizesWalletSlice(t *testing.T) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	balance := broker.NewBalance(nil, nil, nil)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
	))
	instrument := broker.NewInstrument(nil, nil, nil)
	instrument.On([]byte(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5}]}}`,
	))
	price := broker.NewPrice(nil)
	_ = price.RememberFee("LRC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price.TickerAck([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
	))

	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis(nil)
	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action: types.ActionEnter, Symbol: "LRC/USD", AllocationHaircut: 0,
		At: time.Unix(1, 0).UTC(),
	})
	thesis.Holdings.Store("LRC/USD", &types.Holding{
		Symbol: "LRC/USD", Status: types.PENDING,
	})

	if err := allocator.Allocate(thesis); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if thesis.Decisions[0].ProposedQuantity == nil ||
		thesis.Decisions[0].ProposedQuantity.Float64() < 50 {
		t.Fatalf("want qty >= 50, got %v (%s)",
			thesis.Decisions[0].ProposedQuantity, thesis.Decisions[0].Reason)
	}

	if value, ok := thesis.Holdings.Load("LRC/USD"); !ok ||
		value.(*types.Holding).Qty == nil {
		t.Fatal("want holding qty written")
	}
}

func TestAllocatorScalesByRisk(t *testing.T) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	run := func(risk float64) float64 {
		balance := broker.NewBalance(nil, nil, nil)
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
		))
		instrument := broker.NewInstrument(nil, nil, nil)
		instrument.On([]byte(
			`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
				`"symbol":"MATIC/USD","base":"MATIC","quote":"USD","status":"online",` +
				`"qty_min":4,"qty_increment":0.00000001,"qty_precision":8,` +
				`"cost_min":"0.43","cost_precision":6,"tick_size":"0.0001",` +
				`"price_increment":"0.0001","price_precision":4}]}}`,
		))
		price := broker.NewPrice(nil)
		_ = price.RememberFee("MATIC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		price.TickerAck([]byte(
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"MATIC/USD","last":"0.10035","bid":"0.10025","ask":"0.10035"}]}`,
		))
		allocator := NewAllocator(context.Background(), balance, instrument, price)
		thesis := types.NewThesis(nil)
		thesis.Decisions = append(thesis.Decisions, types.Decision{
			Action: types.ActionEnter, Symbol: "MATIC/USD", AllocationHaircut: risk,
		})
		thesis.Holdings.Store("MATIC/USD", &types.Holding{
			Symbol: "MATIC/USD", Status: types.PENDING,
		})
		if err := allocator.Allocate(thesis); err != nil {
			t.Fatalf("Allocate risk=%v: %v", risk, err)
		}
		if thesis.Decisions[0].ProposedQuantity == nil {
			t.Fatalf("unsized risk=%v (%s)", risk, thesis.Decisions[0].Reason)
		}
		return thesis.Decisions[0].ProposedQuantity.Float64()
	}

	if run(0.5) >= run(0) {
		t.Fatal("want higher risk to shrink quantity")
	}
}

func TestAllocatorBailsWhenMinimumExceedsWalletSlice(t *testing.T) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	balance := broker.NewBalance(nil, nil, nil)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"10","available":"10","reserved":"0"}]}`,
	))
	instrument := broker.NewInstrument(nil, nil, nil)
	instrument.On([]byte(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5}]}}`,
	))
	price := broker.NewPrice(nil)
	_ = price.RememberFee("LRC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price.TickerAck([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
	))

	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis(nil)
	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action: types.ActionEnter, Symbol: "LRC/USD",
	})
	thesis.Holdings.Store("LRC/USD", &types.Holding{
		Symbol: "LRC/USD", Status: types.PENDING,
	})

	if err := allocator.Allocate(thesis); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if _, ok := thesis.Holdings.Load("LRC/USD"); ok {
		t.Fatal("want holding removed when minimum exceeds wallet slice")
	}
}

func BenchmarkAllocatorAllocate(b *testing.B) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	b.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	balance := broker.NewBalance(nil, nil, nil)
	instrument := broker.NewInstrument(nil, nil, nil)
	instrument.On([]byte(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5}]}}`,
	))
	price := broker.NewPrice(nil)
	_ = price.RememberFee("LRC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price.TickerAck([]byte(
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
	))
	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis(nil)
	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action: types.ActionEnter, Symbol: "LRC/USD",
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
		))
		thesis.Holdings.Store("LRC/USD", &types.Holding{
			Symbol: "LRC/USD", Status: types.PENDING,
		})
		if err := allocator.Allocate(thesis); err != nil {
			b.Fatal(err)
		}
	}
}
