package trader

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func TestOpportunityArtifactsReachPrivateOrderDispatch(t *testing.T) {
	oldQuote := viper.GetString("market.quote_currency")
	oldMaxAge := viper.GetString("market.story.measurement_max_age")
	viper.Set("market.quote_currency", "USD")
	viper.Set("market.story.measurement_max_age", "1h")
	t.Cleanup(func() {
		viper.Set("market.quote_currency", oldQuote)
		viper.Set("market.story.measurement_max_age", oldMaxAge)
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pool := qpool.NewQ[any](ctx, 4, 8, nil)
	story := market.NewStory(ctx, pool)
	t.Cleanup(func() { _ = story.Close() })

	desk := broker.NewDesk(ctx, pool, dmt.NewTree(""))
	t.Cleanup(func() { _ = desk.Close() })

	orders := make(chan *datura.Artifact, 1)
	pool.Subscribe("kraken:private", func(artifact *datura.Artifact) error {
		orders <- artifact
		return nil
	})

	balances := tradePathBalances("USD", 1000, "ALGO", 0)
	t.Cleanup(balances.Release)
	if err := pool.CreateBroadcastGroup("balances").Send(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tradePathTicker("ALGO/USD", 99, 101, 100)
	t.Cleanup(ticker.Release)
	if err := pool.CreateBroadcastGroup("ticker").Send(ticker); err != nil {
		t.Fatal(err)
	}

	at := time.Now().UTC()
	measurements := []*datura.Artifact{
		tradePathMeasurement(logic.SourcePumpDump, "ALGO/USD", logic.CategoryOrganicTrend, 0.95, at),
		tradePathMeasurement(logic.SourceSentiment, "ALGO/USD", logic.CategoryRiskOnSurge, 0.80, at),
		tradePathMeasurement(logic.SourceExhaustion, "ALGO/USD", logic.CategoryThermalExhaustion, 0.70, at),
		tradePathMeasurement(logic.SourceLiquidity, "ALGO/USD", logic.CategoryRobustLiquidity, 0.70, at),
	}
	for _, measurement := range measurements {
		t.Cleanup(measurement.Release)
	}

	story.Update(measurements)
	actions := story.Actions(balances)
	if len(actions) != 1 {
		t.Fatalf("story actions = %d, want 1", len(actions))
	}
	if actionType := datura.Peek[string](actions[0], "type"); actionType != "market" {
		t.Fatalf("action type = %q, want market", actionType)
	}
	if side := datura.Peek[string](actions[0], "side"); side != "buy" {
		t.Fatalf("action side = %q, want buy", side)
	}

	chosen, verdicts := NewDecider().choose(measurements, actions, balances)
	if len(chosen) != 1 {
		t.Fatalf("chosen actions = %d, want 1; verdicts=%v", len(chosen), verdicts)
	}
	if verdict := datura.Peek[string](chosen[0], "decision", "verdict"); verdict != "allow" {
		t.Fatalf("decision verdict = %q, want allow", verdict)
	}

	allocator := &Allocator{fraction: 0.05, quote: "USD"}
	allowed := allocator.Allowed(chosen, balances)
	if len(allowed) != 1 {
		t.Fatalf("allowed actions = %d, want 1", len(allowed))
	}
	if !datura.Peek[bool](allowed[0], "risk", "stamped") {
		t.Fatal("allocator did not stamp broker risk gate")
	}

	if err := desk.Update(allowed); err != nil {
		t.Fatal(err)
	}

	order := receiveTradePathOrder(t, orders)
	if method := datura.Peek[string](order, "method"); method != "add_order" {
		t.Fatalf("order method = %q, want add_order", method)
	}
	if symbol := datura.Peek[string](order, "params", "symbol"); symbol != "ALGO/USD" {
		t.Fatalf("order symbol = %q, want ALGO/USD", symbol)
	}
	if side := datura.Peek[string](order, "params", "side"); side != "buy" {
		t.Fatalf("order side = %q, want buy", side)
	}
	if orderType := datura.Peek[string](order, "params", "order_type"); orderType != "market" {
		t.Fatalf("order type = %q, want market", orderType)
	}

	expectedFraction := 0.05 * datura.Peek[float64](allowed[0], "confidence")
	expectedQty := 1000 * expectedFraction / 101
	if qty := datura.Peek[float64](order, "params", "order_qty"); math.Abs(qty-expectedQty) > 1e-12 {
		t.Fatalf("order_qty = %v, want %v", qty, expectedQty)
	}
}

func tradePathMeasurement(
	source logic.SourceType,
	symbol string,
	category logic.CategoryType,
	confidence float64,
	at time.Time,
) *datura.Artifact {
	index := logic.CategoryIndex(category)
	artifact := datura.Acquire("test", datura.APPJSON)
	artifact.WithRole("measurement")
	artifact.WithScope(symbol)
	_ = artifact.SetOrigin(string(source))
	artifact.SetTimestamp(at.UnixNano())
	artifact.WithPayload([]byte(`{}`))
	artifact.Poke(datura.Map[float64]{
		fmt.Sprintf("category.%d", index): 1,
		"value":                           float64(index),
		"confidence":                      confidence,
		"strength":                        confidence,
	}, "output")

	return artifact
}

func tradePathBalances(quote string, quoteBalance float64, base string, baseBalance float64) *datura.Artifact {
	return datura.Acquire("test", datura.APPJSON).
		WithDestination("balances").
		WithRole("balances").
		WithScope("balances").
		WithPayload(datura.Map[any]{
			"channel": "balances",
			"type":    "snapshot",
			"data": []datura.Map[any]{
				{"asset": quote, "balance": quoteBalance},
				{"asset": base, "balance": baseBalance},
			},
		}.Marshal())
}

func tradePathTicker(symbol string, bid float64, ask float64, last float64) *datura.Artifact {
	return datura.Acquire("test", datura.APPJSON).
		WithDestination("ticker").
		WithRole("ticker").
		WithScope(symbol).
		WithPayload(datura.Map[any]{
			"channel": "ticker",
			"type":    "update",
			"data": []datura.Map[any]{
				{"symbol": symbol, "bid": bid, "ask": ask, "last": last},
			},
		}.Marshal())
}

func receiveTradePathOrder(t *testing.T, orders <-chan *datura.Artifact) *datura.Artifact {
	t.Helper()

	select {
	case order := <-orders:
		return order
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for private order")
		return nil
	}
}
