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
	order := addOrderWithParams("BTC/USD", "buy", "market", 0.1, datura.Map[any]{
		"setup_key": "fluid|laminar|buy|market",
	})
	out := orders.Send(order)

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
	if setupKey := datura.Peek[string](out, "data", 0, "setup_key"); setupKey != "fluid|laminar|buy|market" {
		t.Fatalf("setup_key = %q, want fluid|laminar|buy|market", setupKey)
	}
}

func TestOrdersSendRestsNonMarketableLimitOrder(t *testing.T) {
	setFillConfig()

	tree := dmt.NewTree("")
	insertStampedTicker(tree, "BTC/USD", time.Now())
	insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

	orders := NewOrdersWithTree(context.Background(), nil, tree)
	order := addOrderWithParams("BTC/USD", "buy", "limit", 0.1, datura.Map[any]{
		"limit_price": 99.0,
		"setup_key":   "fluid|laminar|buy|limit",
	})

	out := orders.Send(order)
	if out == nil {
		t.Fatal("expected open order artifact")
	}
	if channel := datura.Peek[string](out, "channel"); channel != "orders" {
		t.Fatalf("channel = %q, want orders", channel)
	}
	if status := datura.Peek[string](out, "data", 0, "order_status"); status != "open" {
		t.Fatalf("order_status = %q, want open", status)
	}
	if price := datura.Peek[float64](out, "data", 0, "last_price"); price != 0 {
		t.Fatalf("resting order carried fill price %v", price)
	}
	if setupKey := datura.Peek[string](out, "data", 0, "setup_key"); setupKey != "fluid|laminar|buy|limit" {
		t.Fatalf("setup_key = %q, want fluid|laminar|buy|limit", setupKey)
	}
}

func TestOrdersSendRestsProtectiveTrailingStop(t *testing.T) {
	setFillConfig()

	tree := dmt.NewTree("")
	insertStampedTicker(tree, "BTC/USD", time.Now())
	insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

	orders := NewOrdersWithTree(context.Background(), nil, tree)
	order := addOrderWithParams("BTC/USD", "sell", "trailing-stop", 0.1, datura.Map[any]{
		"trailing_stop": 100.0,
	})

	out := orders.Send(order)
	if out == nil {
		t.Fatal("expected open order artifact")
	}
	if channel := datura.Peek[string](out, "channel"); channel != "orders" {
		t.Fatalf("channel = %q, want orders", channel)
	}
	if status := datura.Peek[string](out, "data", 0, "order_status"); status != "open" {
		t.Fatalf("order_status = %q, want open", status)
	}
	if price := datura.Peek[float64](out, "data", 0, "last_price"); price != 0 {
		t.Fatalf("resting protective order carried fill price %v", price)
	}
}

func addOrder(symbol string, side string, orderType string, qty float64) *datura.Artifact {
	return addOrderWithParams(symbol, side, orderType, qty, nil)
}

func addOrderWithParams(
	symbol string,
	side string,
	orderType string,
	qty float64,
	extra datura.Map[any],
) *datura.Artifact {
	params := datura.Map[any]{
		"symbol":     symbol,
		"side":       side,
		"order_type": orderType,
		"order_qty":  qty,
		"cl_ord_id":  "test-order",
	}
	for key, value := range extra {
		params[key] = value
	}

	return datura.Acquire("broker", datura.APPJSON).
		WithRole("orders").
		WithScope(symbol).
		WithPayload(datura.Map[any]{
			"method": "add_order",
			"params": params,
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
