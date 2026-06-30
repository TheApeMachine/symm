package response

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestOrdersSendRejectsStalePaperQuoteBeforeFill(t *testing.T) {
	setFillConfig()
	viper.Set("trading.max_quote_age", time.Second)

	tree := dmt.NewTree("")
	insertStampedTicker(tree, "BTC/USD", time.Now().Add(-time.Minute))
	insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

	orders := NewOrdersWithTree(context.Background(), nil, tree)
	out := orders.Send(addOrder("BTC/USD", "buy", "market", 0.1))

	if out == nil {
		t.Fatal("expected rejected execution artifact")
	}
	if status := datura.Peek[string](out, "data", 0, "order_status"); status != "rejected" {
		t.Fatalf("order_status = %q, want rejected", status)
	}
	if execType := datura.Peek[string](out, "data", 0, "exec_type"); execType != "rejected" {
		t.Fatalf("exec_type = %q, want rejected", execType)
	}
	if reason := datura.Peek[string](out, "data", 0, "reject_reason"); !strings.Contains(reason, "stale quote") {
		t.Fatalf("reject_reason = %q, want stale quote", reason)
	}
	if price := datura.Peek[float64](out, "data", 0, "last_price"); price != 0 {
		t.Fatalf("rejected execution carried fill price %v", price)
	}
}

func TestOrdersSendUsesSimulatorFillPrice(t *testing.T) {
	setFillConfig()

	tree := dmt.NewTree("")
	insertStampedTicker(tree, "BTC/USD", time.Now())
	insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

	orders := NewOrdersWithTree(context.Background(), nil, tree)
	out := orders.Send(addOrder("BTC/USD", "buy", "market", 0.1))

	if out == nil {
		t.Fatal("expected filled execution artifact")
	}
	if status := datura.Peek[string](out, "data", 0, "order_status"); status != "filled" {
		t.Fatalf("order_status = %q, want filled", status)
	}
	if price := datura.Peek[float64](out, "data", 0, "last_price"); price <= 0 {
		t.Fatalf("last_price = %v, want priced fill", price)
	}
	if fee := datura.Peek[float64](out, "data", 0, "fee"); fee <= 0 {
		t.Fatalf("fee = %v, want simulator fee", fee)
	}
}

func addOrder(symbol string, side string, orderType string, qty float64) *datura.Artifact {
	return datura.Acquire("broker", datura.APPJSON).
		WithRole("orders").
		WithScope(symbol).
		WithPayload(datura.Map[any]{
			"method": "add_order",
			"params": datura.Map[any]{
				"symbol":     symbol,
				"side":       side,
				"order_type": orderType,
				"order_qty":  qty,
				"cl_ord_id":  "test-order",
			},
		}.Marshal())
}

func insertStampedTicker(tree *dmt.Tree, symbol string, stamp time.Time) {
	artifact := datura.Acquire("test", datura.Artifact_Type_json).
		WithRole("ticker").
		WithScope(symbol).
		WithPayload([]byte(fmt.Sprintf(
			`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":100,"bid":99.5,"ask":100.5}]}`,
			symbol,
		)))
	artifact.SetTimestamp(stamp.UnixNano())

	tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
}
