package trader

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func testAllocator(fraction float64) *Allocator {
	return &Allocator{
		fraction: fraction,
		quote:    "USD",
	}
}

func testRiskConfig(t *testing.T) {
	t.Helper()
	previousQuote := viper.GetString("market.quote_currency")
	t.Cleanup(func() {
		viper.Set("market.quote_currency", previousQuote)
	})
	setRiskConfig()
}

func setRiskConfig() {
	viper.Set("market.quote_currency", "USD")
	viper.Set("trading.max_concurrent_positions", 4)
	viper.Set("trading.slots.normal", 4)
	viper.Set("trading.entry.opportunity_slot_count", 1)
	viper.Set("trading.edge_min_bps", 10.0)
	viper.Set("trading.paper.taker_fee_bps", 0.0)
	viper.Set("trading.paper.slippage_bps", 0.0)
	viper.Set("live.max_order_notional", 0.0)
	viper.Set("live.max_daily_loss", 0.0)
	viper.Set("trading.realized_daily_loss", 0.0)
	viper.Set("trading.daily_loss", 0.0)
	viper.Set("live.realized_daily_loss", 0.0)
	viper.Set("live.daily_loss", 0.0)
}

func allocationAction(symbol string, score, confidence float64) *datura.Artifact {
	return datura.Acquire("story", datura.APPJSON).
		WithRole("buy").
		WithScope(symbol).
		Poke(string(logic.ActionMarket), "type").
		Poke(string(logic.SideBuy), "side").
		Poke(score, "decision", "score").
		Poke(confidence, "decision", "confidence")
}

func allocationExit(symbol string) *datura.Artifact {
	return datura.Acquire("story", datura.APPJSON).
		WithRole("sell").
		WithScope(symbol).
		Poke(string(logic.ActionSettlePosition), "type").
		Poke(string(logic.SideSell), "side")
}

func allocationBalancesUSD(usd float64, rows ...balanceRow) *datura.Artifact {
	data := []datura.Map[any]{{"asset": "USD", "balance": usd}}
	for _, row := range rows {
		data = append(data, datura.Map[any]{
			"asset":   row.Asset,
			"balance": row.Balance,
		})
	}

	return datura.Acquire("test", datura.APPJSON).
		WithRole("balances").
		WithScope("balances").
		WithPayload(datura.Map[any]{
			"channel": "balances",
			"type":    "snapshot",
			"data":    data,
		}.Marshal())
}

func TestAllocatorAllowedRiskGates(t *testing.T) {
	testRiskConfig(t)
	allocator := testAllocator(0.05)
	balances := allocationBalancesUSD(200)
	t.Cleanup(balances.Release)

	actions := []*datura.Artifact{
		allocationAction("LOW/USD", 0.1, 1.0),
		allocationAction("HIGH/USD", 0.9, 1.0),
		allocationAction("MID/USD", 0.5, 1.0),
	}

	out := allocator.Allowed(actions, balances)

	if len(out) != 3 {
		t.Fatalf("actions = %d, want 3", len(out))
	}
	if got := datura.Peek[string](out[0], "scope"); got != "HIGH/USD" {
		t.Fatalf("first symbol = %s, want HIGH/USD", got)
	}
	for _, action := range out {
		if !datura.Peek[bool](action, "allowed") {
			t.Fatalf("expected action allowed: %s reason=%s", datura.Peek[string](action, "scope"), datura.Peek[string](action, "risk", "reason"))
		}
		if notional := datura.Peek[float64](action, "notional"); notional != 10 {
			t.Fatalf("notional = %v, want 10", notional)
		}
	}
}

func TestAllocatorRejectsHeldSymbols(t *testing.T) {
	testRiskConfig(t)
	allocator := testAllocator(0.05)
	balances := allocationBalancesUSD(200, balanceRow{Asset: "BTC", Balance: 1})
	t.Cleanup(balances.Release)

	out := allocator.Allowed([]*datura.Artifact{
		allocationAction("BTC/USD", 0.9, 1),
		allocationAction("ETH/USD", 0.8, 1),
	}, balances)

	if datura.Peek[bool](out[0], "allowed") {
		t.Fatal("held BTC entry was allowed")
	}
	if reason := datura.Peek[string](out[0], "risk", "reason"); reason != "held" {
		t.Fatalf("held reason = %q", reason)
	}
	if !datura.Peek[bool](out[1], "allowed") {
		t.Fatalf("unheld ETH was blocked: %s", datura.Peek[string](out[1], "risk", "reason"))
	}
}

func TestAllocatorRejectsSlotExhaustionAndAdmitsOpportunitySlot(t *testing.T) {
	testRiskConfig(t)
	viper.Set("trading.slots.normal", 1)
	viper.Set("trading.entry.opportunity_slot_count", 1)
	viper.Set("trading.max_concurrent_positions", 3)

	allocator := testAllocator(0.05)
	balances := allocationBalancesUSD(200)
	t.Cleanup(balances.Release)

	normalHigh := allocationAction("HIGH/USD", 0.9, 1)
	normalLow := allocationAction("LOW/USD", 0.8, 1)
	opportunity := allocationAction("OPP/USD", 0.7, 1).
		Poke(true, "opportunity_slot")

	out := allocator.Allowed([]*datura.Artifact{normalLow, opportunity, normalHigh}, balances)

	if !datura.Peek[bool](out[0], "allowed") {
		t.Fatalf("highest normal should be allowed: %s", datura.Peek[string](out[0], "risk", "reason"))
	}
	if reason := datura.Peek[string](out[1], "risk", "reason"); reason != "slot_exhausted" {
		t.Fatalf("second normal reason = %q, want slot_exhausted", reason)
	}
	if !datura.Peek[bool](out[2], "allowed") {
		t.Fatalf("opportunity slot should be allowed: %s", datura.Peek[string](out[2], "risk", "reason"))
	}
}

func TestAllocatorRejectsMaxNotionalAndDailyLoss(t *testing.T) {
	testRiskConfig(t)
	allocator := testAllocator(0.05)
	balances := allocationBalancesUSD(200)
	t.Cleanup(balances.Release)

	viper.Set("live.max_order_notional", 5.0)
	out := allocator.Allowed([]*datura.Artifact{allocationAction("CAP/USD", 1, 1)}, balances)
	if reason := datura.Peek[string](out[0], "risk", "reason"); reason != "max_order_notional" {
		t.Fatalf("notional cap reason = %q", reason)
	}

	testRiskConfig(t)
	viper.Set("live.max_daily_loss", 25.0)
	viper.Set("trading.realized_daily_loss", 25.0)
	out = allocator.Allowed([]*datura.Artifact{allocationAction("LOSS/USD", 1, 1)}, balances)
	if reason := datura.Peek[string](out[0], "risk", "reason"); reason != "max_daily_loss" {
		t.Fatalf("daily loss reason = %q", reason)
	}
}

func TestAllocatorRejectsMaxPositionsWithPendingAndPreservesExits(t *testing.T) {
	testRiskConfig(t)
	viper.Set("trading.max_concurrent_positions", 2)

	allocator := testAllocator(0.05)
	allocator.SetPendingEntries(1)
	balances := allocationBalancesUSD(200, balanceRow{Asset: "BTC", Balance: 1})
	t.Cleanup(balances.Release)

	out := allocator.Allowed([]*datura.Artifact{
		allocationAction("ETH/USD", 1, 1),
		allocationExit("BTC/USD"),
	}, balances)

	if got := datura.Peek[string](out[0], "type"); got != string(logic.ActionSettlePosition) {
		t.Fatalf("first action = %s, want exit first", got)
	}
	if !datura.Peek[bool](out[0], "allowed") {
		t.Fatal("exit was blocked")
	}
	if reason := datura.Peek[string](out[1], "risk", "reason"); reason != "max_positions" {
		t.Fatalf("entry reason = %q, want max_positions", reason)
	}
}

func TestAllocatorCalculate(t *testing.T) {
	allocator := testAllocator(0.05)
	if got := allocator.calculate(0.6); got != 0.03 {
		t.Fatalf("calculate = %v, want 0.03", got)
	}
	if got := allocator.calculate(0); got != 0 {
		t.Fatalf("zero confidence calculate = %v, want 0", got)
	}
}

func BenchmarkAllocatorAllowed(benchmark *testing.B) {
	setRiskConfig()
	allocator := testAllocator(0.05)
	balances := allocationBalancesUSD(200)
	benchmark.Cleanup(balances.Release)

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for index := 0; index < benchmark.N; index++ {
		actions := make([]*datura.Artifact, 16)
		for actionIndex := range actions {
			actions[actionIndex] = allocationAction(
				fmt.Sprintf("ASSET%d/USD", actionIndex),
				float64(len(actions)-actionIndex),
				0.5,
			)
		}

		allocator.Allowed(actions, balances)
	}
}
