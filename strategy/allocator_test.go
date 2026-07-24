package strategy

import (
	"github.com/theapemachine/symm/config"
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func allocatorWire(
	instrumentFrame string,
	tickerFrame string,
	fees map[string]float64,
) (*broker.Balance, *broker.Instrument, *broker.Price) {
	balance := broker.NewBalance(nil, nil, nil, config.Fixture().Market)
	price := broker.NewPrice(nil)

	for symbol, fee := range fees {
		_ = price.RememberFee(symbol, kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(fee),
		})
	}

	if tickerFrame != "" {
		price.TickerAck(kraken.NewTicker([]byte(tickerFrame)))
	}

	instrument := broker.NewInstrument(nil, price, nil, config.Fixture().Market)

	if instrumentFrame != "" {
		frame := kraken.NewInstrument([]byte(instrumentFrame))

		for index := range frame.Data.Pairs {
			instrument.Remember(frame.Data.Pairs[index])
		}
	}

	return balance, instrument, price
}

func TestAllocatorSizesWalletSlice(t *testing.T) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	balance := broker.NewBalance(nil, nil, nil, config.Fixture().Market)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
	))
	_, instrument, price := allocatorWire(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5}]}}`,
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
		map[string]float64{"LRC/USD": 0.26},
	)

	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis()
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

	// 5000 USD * 0.20 wallet slice at ask 0.05, after the 0.26% taker fee
	// haircut on executable notional → exactly 19948 LRC.
	if thesis.Decisions[0].ProposedQuantity == nil ||
		thesis.Decisions[0].ProposedQuantity.Float64() != 19948 {
		t.Fatalf("want qty 19948, got %v (%s)",
			thesis.Decisions[0].ProposedQuantity, thesis.Decisions[0].Reason)
	}

	if thesis.Decisions[0].ProposedNotional == nil ||
		thesis.Decisions[0].ProposedNotional.Float64() != 999.99324 {
		t.Fatalf("want notional 999.99324, got %v", thesis.Decisions[0].ProposedNotional)
	}

	if value, ok := thesis.Holdings.Load("LRC/USD"); !ok ||
		value.(*types.Holding).Qty == nil ||
		value.(*types.Holding).Qty.Float64() != 19948 {
		t.Fatal("want holding qty 19948 written")
	}
}

func TestAllocatorScalesByRisk(t *testing.T) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	run := func(risk float64) float64 {
		balance := broker.NewBalance(nil, nil, nil, config.Fixture().Market)
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
		))
		_, instrument, price := allocatorWire(
			`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
				`"symbol":"MATIC/USD","base":"MATIC","quote":"USD","status":"online",` +
				`"qty_min":4,"qty_increment":0.00000001,"qty_precision":8,` +
				`"cost_min":"0.43","cost_precision":6,"tick_size":"0.0001",` +
				`"price_increment":"0.0001","price_precision":4}]}}`,
			`{"channel":"ticker","type":"update","data":[{` +
				`"symbol":"MATIC/USD","last":"0.10035","bid":"0.10025","ask":"0.10035"}]}`,
			map[string]float64{"MATIC/USD": 0.26},
		)
		allocator := NewAllocator(context.Background(), balance, instrument, price)
		thesis := types.NewThesis()
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

	balance := broker.NewBalance(nil, nil, nil, config.Fixture().Market)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"10","available":"10","reserved":"0"}]}`,
	))
	_, instrument, price := allocatorWire(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5}]}}`,
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
		map[string]float64{"LRC/USD": 0.26},
	)

	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis()
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

func TestAllocatorTransactionLocalBudget(t *testing.T) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	balance := broker.NewBalance(nil, nil, nil, config.Fixture().Market)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
	))
	_, instrument, price := allocatorWire(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5},{` +
			`"symbol":"MATIC/USD","base":"MATIC","quote":"USD","status":"online",` +
			`"qty_min":4,"qty_increment":0.00000001,"qty_precision":8,` +
			`"cost_min":"0.43","cost_precision":6,"tick_size":"0.0001",` +
			`"price_increment":"0.0001","price_precision":4}]}}`,
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"},{` +
			`"symbol":"MATIC/USD","last":"0.10035","bid":"0.10025","ask":"0.10035"}]}`,
		map[string]float64{"LRC/USD": 0.26, "MATIC/USD": 0.26},
	)

	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis()
	thesis.Decisions = append(thesis.Decisions,
		types.Decision{
			Action: types.ActionEnter, Symbol: "LRC/USD",
			AllocationHaircut: 0, At: time.Unix(1, 0).UTC(),
		},
		types.Decision{
			Action: types.ActionEnter, Symbol: "MATIC/USD",
			AllocationHaircut: 0, At: time.Unix(2, 0).UTC(),
		},
	)
	thesis.Holdings.Store("LRC/USD", &types.Holding{Symbol: "LRC/USD", Status: types.PENDING})
	thesis.Holdings.Store("MATIC/USD", &types.Holding{Symbol: "MATIC/USD", Status: types.PENDING})

	if err := allocator.Allocate(thesis); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if thesis.Decisions[0].ProposedQuantity == nil ||
		thesis.Decisions[1].ProposedQuantity == nil {
		t.Fatalf("want both decisions sized: %q %q",
			thesis.Decisions[0].Reason, thesis.Decisions[1].Reason)
	}

	if thesis.Decisions[1].ProposedNotional.Cmp(thesis.Decisions[0].ProposedNotional) >= 0 {
		t.Fatalf("want second entry smaller after budget reservation: first=%v second=%v",
			thesis.Decisions[0].ProposedNotional, thesis.Decisions[1].ProposedNotional)
	}
}

func TestAllocatorHonorsRotationNotional(t *testing.T) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	balance := broker.NewBalance(nil, nil, nil, config.Fixture().Market)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
	))
	_, instrument, price := allocatorWire(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5}]}}`,
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
		map[string]float64{"LRC/USD": 0.26},
	)

	rotationBudget := decimal.NewFromFloat64(250)
	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis()
	thesis.Decisions = append(thesis.Decisions, types.Decision{
		Action:           types.ActionEnter,
		Symbol:           "LRC/USD",
		Cause:            "rotation",
		ProposedNotional: rotationBudget.Copy(),
		AllocationHaircut: 0,
		At:               time.Unix(3, 0).UTC(),
	})
	thesis.Holdings.Store("LRC/USD", &types.Holding{Symbol: "LRC/USD", Status: types.PENDING})

	if err := allocator.Allocate(thesis); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if thesis.Decisions[0].ProposedQuantity == nil {
		t.Fatalf("rotation unsized: %s", thesis.Decisions[0].Reason)
	}

	if thesis.Decisions[0].ProposedNotional.Cmp(rotationBudget) > 0 {
		t.Fatalf("rotation cost %v exceeds redeploy budget %v",
			thesis.Decisions[0].ProposedNotional, rotationBudget)
	}

	walletSlice := decimal.NewFromFloat64(5000 * 0.20)
	if thesis.Decisions[0].ProposedNotional.Cmp(walletSlice) >= 0 {
		t.Fatalf("rotation used wallet slice %v instead of redeploy budget",
			thesis.Decisions[0].ProposedNotional)
	}
}

func TestAllocatorRejectsInvalidHaircutPerDecision(t *testing.T) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	t.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	balance := broker.NewBalance(nil, nil, nil, config.Fixture().Market)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"5000","available":"5000","reserved":"0"}]}`,
	))
	_, instrument, price := allocatorWire(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5},{` +
			`"symbol":"MATIC/USD","base":"MATIC","quote":"USD","status":"online",` +
			`"qty_min":4,"qty_increment":0.00000001,"qty_precision":8,` +
			`"cost_min":"0.43","cost_precision":6,"tick_size":"0.0001",` +
			`"price_increment":"0.0001","price_precision":4}]}}`,
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"},{` +
			`"symbol":"MATIC/USD","last":"0.10035","bid":"0.10025","ask":"0.10035"}]}`,
		map[string]float64{"LRC/USD": 0.26, "MATIC/USD": 0.26},
	)

	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis()
	thesis.Decisions = append(thesis.Decisions,
		types.Decision{
			Action: types.ActionEnter, Symbol: "LRC/USD",
			AllocationHaircut: 1.5,
		},
		types.Decision{
			Action: types.ActionEnter, Symbol: "MATIC/USD",
			AllocationHaircut: 0,
		},
	)
	thesis.Holdings.Store("LRC/USD", &types.Holding{Symbol: "LRC/USD", Status: types.PENDING})
	thesis.Holdings.Store("MATIC/USD", &types.Holding{Symbol: "MATIC/USD", Status: types.PENDING})

	if err := allocator.Allocate(thesis); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if thesis.Decisions[0].ProposedQuantity != nil {
		t.Fatal("want invalid haircut decision rejected")
	}

	if thesis.Decisions[1].ProposedQuantity == nil {
		t.Fatalf("want later decision still sized after reject: %s",
			thesis.Decisions[1].Reason)
	}
}

func BenchmarkAllocatorAllocate(b *testing.B) {
	previous := viper.GetFloat64("trading.allocation.max_fraction")
	viper.Set("trading.allocation.max_fraction", 0.20)
	b.Cleanup(func() { viper.Set("trading.allocation.max_fraction", previous) })
	viper.Set("market.quote_currency", "USD")

	balance := broker.NewBalance(nil, nil, nil, config.Fixture().Market)
	_, instrument, price := allocatorWire(
		`{"channel":"instrument","type":"snapshot","data":{"pairs":[{` +
			`"symbol":"LRC/USD","base":"LRC","quote":"USD","status":"online",` +
			`"qty_min":50,"qty_increment":1,"qty_precision":0,` +
			`"cost_min":"0.5","cost_precision":5,"tick_size":"0.00001",` +
			`"price_increment":"0.00001","price_precision":5}]}}`,
		`{"channel":"ticker","type":"update","data":[{` +
			`"symbol":"LRC/USD","last":"0.05","bid":"0.04999","ask":"0.05"}]}`,
		map[string]float64{"LRC/USD": 0.26},
	)
	allocator := NewAllocator(context.Background(), balance, instrument, price)
	thesis := types.NewThesis()
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
