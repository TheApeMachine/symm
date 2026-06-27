package broker

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func setupLiveDesk(t *testing.T, maxOrderNotional, maxDailyLoss float64) (*Desk, *dmt.Tree, chan *datura.Artifact) {
	t.Helper()
	viper.Reset()
	t.Setenv("SYMM_LIVE", "")
	viper.Set("trading.model", "live")
	viper.Set("market.quote_currency", "USD")
	viper.Set("live.max_order_notional", maxOrderNotional)
	viper.Set("live.max_daily_loss", maxDailyLoss)

	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree("")

	seedBalance(tree, "USD", 1000)
	seedInstrument(tree, "BTC/USD", `{"qty_increment":0.000000000001,"qty_min":0.00000001,"cost_min":0.01}`)

	orders := captureOrders(pool)
	desk := NewDesk(ctx, pool, tree)
	seedTicker(desk, "BTC/USD", 100)

	t.Cleanup(func() {
		_ = desk.Close()
		viper.Reset()
	})

	return desk, tree, orders
}

func buyAction(clOrdID string, qty float64) *datura.Artifact {
	return datura.Acquire("story", datura.APPJSON).
		WithRole("buy").
		WithScope("BTC/USD").
		WithPayload(datura.Map[any]{
			"type":      "market",
			"quantity":  qty,
			"cl_ord_id": clOrdID,
			"offset":    0.05,
		}.Marshal())
}

func sellAction(clOrdID string, qty float64) *datura.Artifact {
	return datura.Acquire("story", datura.APPJSON).
		WithRole("sell").
		WithScope("BTC/USD").
		WithPayload(datura.Map[any]{
			"type":      "market",
			"quantity":  qty,
			"cl_ord_id": clOrdID,
		}.Marshal())
}

func assertNoOrder(t *testing.T, orders chan *datura.Artifact) {
	t.Helper()
	select {
	case order := <-orders:
		t.Fatalf("unexpected order: %v", order)
	default:
	}
}

func diagnosticExists(tree *dmt.Tree, code string) bool {
	for artifact := range tree.Seek([]byte("diagnostic/" + code)) {
		artifact.Release()
		return true
	}

	return false
}

func TestLiveMaxOrderNotionalBlocksEntry(t *testing.T) {
	desk, tree, orders := setupLiveDesk(t, 50, 25)

	err := desk.Update([]*datura.Artifact{buyAction("too-large", 1)})
	if err == nil {
		t.Fatal("expected max notional rejection")
	}

	assertNoOrder(t, orders)
	if !diagnosticExists(tree, "live_max_order_notional") {
		t.Fatal("expected live_max_order_notional diagnostic")
	}
}

func TestLiveDailyLossBlocksNewEntries(t *testing.T) {
	desk, tree, orders := setupLiveDesk(t, 1000, 5)

	if err := desk.Update([]*datura.Artifact{buyAction("open-1", 1)}); err != nil {
		t.Fatalf("open entry: %v", err)
	}
	if order := awaitOrder(orders); order == nil {
		t.Fatal("expected opening buy order")
	}
	if err := desk.onMessage(executionArtifact("open-1", "BTC/USD", "buy", 1, 1, 100, "filled", "trade")); err != nil {
		t.Fatalf("buy execution: %v", err)
	}

	if err := desk.Update([]*datura.Artifact{sellAction("exit-1", 1)}); err != nil {
		t.Fatalf("protective sell before loss limit: %v", err)
	}
	if order := awaitOrder(orders); order == nil {
		t.Fatal("expected protective sell order")
	}
	if err := desk.onMessage(executionArtifact("exit-1", "BTC/USD", "sell", 1, 1, 90, "filled", "trade")); err != nil {
		t.Fatalf("sell execution: %v", err)
	}

	err := desk.Update([]*datura.Artifact{buyAction("blocked", 1)})
	if err == nil {
		t.Fatal("expected daily loss entry rejection")
	}

	assertNoOrder(t, orders)
	if !diagnosticExists(tree, "live_daily_loss_limit") {
		t.Fatal("expected live_daily_loss_limit diagnostic")
	}
}

func TestLiveDailyLossStillAllowsProtectiveExit(t *testing.T) {
	desk, _, orders := setupLiveDesk(t, 1000, 5)

	if err := desk.Update([]*datura.Artifact{buyAction("open-1", 1)}); err != nil {
		t.Fatalf("open entry: %v", err)
	}
	if order := awaitOrder(orders); order == nil {
		t.Fatal("expected opening buy order")
	}
	if err := desk.onMessage(executionArtifact("open-1", "BTC/USD", "buy", 1, 1, 100, "filled", "trade")); err != nil {
		t.Fatalf("buy execution: %v", err)
	}
	if err := desk.onMessage(executionArtifact("loss-1", "BTC/USD", "sell", 1, 1, 90, "filled", "trade")); err != nil {
		t.Fatalf("loss execution: %v", err)
	}

	if err := desk.Update([]*datura.Artifact{sellAction("protective-exit", 1)}); err != nil {
		t.Fatalf("sell should remain allowed after daily loss trip: %v", err)
	}
	if order := awaitOrder(orders); order == nil {
		t.Fatal("expected sell order after daily loss trip")
	}
}
