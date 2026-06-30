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
	if channel := datura.Peek[string](out, "channel"); channel != "executions" {
		t.Fatalf("channel = %q, want executions", channel)
	}
	if status := datura.Peek[string](out, "data", 0, "order_status"); status != "open" {
		t.Fatalf("order_status = %q, want open", status)
	}
	if execType := datura.Peek[string](out, "data", 0, "exec_type"); execType != "new" {
		t.Fatalf("exec_type = %q, want new", execType)
	}
	if price := datura.Peek[float64](out, "data", 0, "last_price"); price != 0 {
		t.Fatalf("resting order carried fill price %v", price)
	}
	if setupKey := datura.Peek[string](out, "data", 0, "setup_key"); setupKey != "fluid|laminar|buy|limit" {
		t.Fatalf("setup_key = %q, want fluid|laminar|buy|limit", setupKey)
	}
	if qty := datura.Peek[float64](out, "data", 0, "qty"); qty != 0.1 {
		t.Fatalf("qty = %v, want 0.1", qty)
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
	if channel := datura.Peek[string](out, "channel"); channel != "executions" {
		t.Fatalf("channel = %q, want executions", channel)
	}
	if status := datura.Peek[string](out, "data", 0, "order_status"); status != "open" {
		t.Fatalf("order_status = %q, want open", status)
	}
	if execType := datura.Peek[string](out, "data", 0, "exec_type"); execType != "new" {
		t.Fatalf("exec_type = %q, want new", execType)
	}
	if price := datura.Peek[float64](out, "data", 0, "last_price"); price != 0 {
		t.Fatalf("resting protective order carried fill price %v", price)
	}
	if trailing := datura.Peek[float64](out, "data", 0, "trailing_offset"); trailing != 100 {
		t.Fatalf("trailing_offset = %v, want 100", trailing)
	}
}

func TestOrdersSendRejectsPaperTradingRateLimit(t *testing.T) {
	setFillConfig()
	setPaperRateLimitTier(t, "starter")

	tree := dmt.NewTree("")
	insertStampedTicker(tree, "BTC/USD", time.Now())
	insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

	orders := NewOrdersWithTree(context.Background(), nil, tree)
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	orders.limits.now = func() time.Time { return now }

	for index := 0; index < 60; index++ {
		out := orders.Send(addOrderWithParams("BTC/USD", "buy", "market", 0.01, datura.Map[any]{
			"cl_ord_id": fmt.Sprintf("burst-%03d", index),
		}))
		if status := datura.Peek[string](out, "data", 0, "order_status"); status != "filled" {
			t.Fatalf("order %d status = %q, want filled", index, status)
		}
	}

	out := orders.Send(addOrderWithParams("BTC/USD", "buy", "market", 0.01, datura.Map[any]{
		"cl_ord_id": "burst-reject",
	}))
	if status := datura.Peek[string](out, "data", 0, "order_status"); status != "rejected" {
		t.Fatalf("order_status = %q, want rejected", status)
	}
	if reason := datura.Peek[string](out, "data", 0, "reject_reason"); reason != paperRateLimitExceeded {
		t.Fatalf("reject_reason = %q, want %q", reason, paperRateLimitExceeded)
	}
}

func TestOrdersSendPaperTradingRateLimitDecays(t *testing.T) {
	setFillConfig()
	setPaperRateLimitTier(t, "starter")

	tree := dmt.NewTree("")
	insertStampedTicker(tree, "BTC/USD", time.Now())
	insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

	orders := NewOrdersWithTree(context.Background(), nil, tree)
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	orders.limits.now = func() time.Time { return now }

	for index := 0; index < 60; index++ {
		orders.Send(addOrderWithParams("BTC/USD", "buy", "market", 0.01, datura.Map[any]{
			"cl_ord_id": fmt.Sprintf("decay-%03d", index),
		}))
	}

	rejected := orders.Send(addOrderWithParams("BTC/USD", "buy", "market", 0.01, datura.Map[any]{
		"cl_ord_id": "decay-reject",
	}))
	if reason := datura.Peek[string](rejected, "data", 0, "reject_reason"); reason != paperRateLimitExceeded {
		t.Fatalf("reject_reason = %q, want %q", reason, paperRateLimitExceeded)
	}

	now = now.Add(2 * time.Second)
	accepted := orders.Send(addOrderWithParams("BTC/USD", "buy", "market", 0.01, datura.Map[any]{
		"cl_ord_id": "decay-accepted",
	}))
	if status := datura.Peek[string](accepted, "data", 0, "order_status"); status != "filled" {
		t.Fatalf("order_status after decay = %q, want filled", status)
	}
}

func TestOrdersSendRejectsPaperOpenOrderLimit(t *testing.T) {
	setFillConfig()
	setPaperRateLimitTier(t, "intermediate")

	tree := dmt.NewTree("")
	insertStampedTicker(tree, "BTC/USD", time.Now())
	insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

	orders := NewOrdersWithTree(context.Background(), nil, tree)
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	orders.limits.now = func() time.Time { return now }

	for index := 0; index < 80; index++ {
		out := orders.Send(addOrderWithParams("BTC/USD", "buy", "limit", 0.01, datura.Map[any]{
			"cl_ord_id":    fmt.Sprintf("open-%03d", index),
			"limit_price":  99.0,
			"trigger_note": "nonmarketable",
		}))
		if status := datura.Peek[string](out, "data", 0, "order_status"); status != "open" {
			t.Fatalf("order %d status = %q, want open", index, status)
		}
	}

	out := orders.Send(addOrderWithParams("BTC/USD", "buy", "limit", 0.01, datura.Map[any]{
		"cl_ord_id":   "open-reject",
		"limit_price": 99.0,
	}))
	if status := datura.Peek[string](out, "data", 0, "order_status"); status != "rejected" {
		t.Fatalf("order_status = %q, want rejected", status)
	}
	if reason := datura.Peek[string](out, "data", 0, "reject_reason"); reason != paperOpenLimitExceeded {
		t.Fatalf("reject_reason = %q, want %q", reason, paperOpenLimitExceeded)
	}
}

func TestOrdersCancelAppliesPenaltyAndFreesPaperOpenSlot(t *testing.T) {
	setFillConfig()
	setPaperRateLimitTier(t, "starter")

	tree := dmt.NewTree("")
	insertStampedTicker(tree, "BTC/USD", time.Now())
	insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

	orders := NewOrdersWithTree(context.Background(), nil, tree)
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	orders.limits.now = func() time.Time { return now }

	open := orders.Send(addOrderWithParams("BTC/USD", "buy", "limit", 0.01, datura.Map[any]{
		"cl_ord_id":   "cancel-open",
		"limit_price": 99.0,
	}))
	orderID := datura.Peek[string](open, "data", 0, "order_id")
	if orderID == "" {
		t.Fatal("open order did not expose order_id")
	}

	canceled := orders.Send(cancelOrder(orderID))
	if status := datura.Peek[string](canceled, "data", 0, "order_status"); status != "canceled" {
		t.Fatalf("cancel status = %q, want canceled", status)
	}
	if execType := datura.Peek[string](canceled, "data", 0, "exec_type"); execType != "canceled" {
		t.Fatalf("cancel exec_type = %q, want canceled", execType)
	}

	for index := 0; index < 51; index++ {
		orders.Send(addOrderWithParams("BTC/USD", "buy", "market", 0.01, datura.Map[any]{
			"cl_ord_id": fmt.Sprintf("after-cancel-%03d", index),
		}))
	}

	rejected := orders.Send(addOrderWithParams("BTC/USD", "buy", "market", 0.01, datura.Map[any]{
		"cl_ord_id": "after-cancel-reject",
	}))
	if reason := datura.Peek[string](rejected, "data", 0, "reject_reason"); reason != paperRateLimitExceeded {
		t.Fatalf("reject_reason = %q, want cancel penalty to contribute %q", reason, paperRateLimitExceeded)
	}
}

func TestOrdersAmendAppliesPaperRatePenalty(t *testing.T) {
	setFillConfig()
	setPaperRateLimitTier(t, "starter")

	tree := dmt.NewTree("")
	insertStampedTicker(tree, "BTC/USD", time.Now())
	insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

	orders := NewOrdersWithTree(context.Background(), nil, tree)
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	orders.limits.now = func() time.Time { return now }

	open := orders.Send(addOrderWithParams("BTC/USD", "buy", "limit", 0.01, datura.Map[any]{
		"cl_ord_id":   "amend-open",
		"limit_price": 99.0,
	}))
	orderID := datura.Peek[string](open, "data", 0, "order_id")
	if orderID == "" {
		t.Fatal("open order did not expose order_id")
	}

	amended := orders.Send(amendOrder(orderID, 98.5))
	if status := datura.Peek[string](amended, "data", 0, "order_status"); status != "open" {
		t.Fatalf("amend status = %q, want open", status)
	}
	if execType := datura.Peek[string](amended, "data", 0, "exec_type"); execType != "amended" {
		t.Fatalf("amend exec_type = %q, want amended", execType)
	}
	if limit := datura.Peek[float64](amended, "data", 0, "limit_price"); limit != 98.5 {
		t.Fatalf("limit_price = %v, want amended 98.5", limit)
	}

	for index := 0; index < 55; index++ {
		orders.Send(addOrderWithParams("BTC/USD", "buy", "market", 0.01, datura.Map[any]{
			"cl_ord_id": fmt.Sprintf("after-amend-%03d", index),
		}))
	}

	rejected := orders.Send(addOrderWithParams("BTC/USD", "buy", "market", 0.01, datura.Map[any]{
		"cl_ord_id": "after-amend-reject",
	}))
	if reason := datura.Peek[string](rejected, "data", 0, "reject_reason"); reason != paperRateLimitExceeded {
		t.Fatalf("reject_reason = %q, want amend penalty to contribute %q", reason, paperRateLimitExceeded)
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

func cancelOrder(orderID string) *datura.Artifact {
	return datura.Acquire("broker", datura.APPJSON).
		WithRole("orders").
		WithPayload(datura.Map[any]{
			"method": "cancel_order",
			"params": datura.Map[any]{
				"order_id": orderID,
			},
		}.Marshal())
}

func amendOrder(orderID string, limitPrice float64) *datura.Artifact {
	return datura.Acquire("broker", datura.APPJSON).
		WithRole("orders").
		WithPayload(datura.Map[any]{
			"method": "amend_order",
			"params": datura.Map[any]{
				"order_id":    orderID,
				"limit_price": limitPrice,
			},
		}.Marshal())
}

func setPaperRateLimitTier(t *testing.T, tier string) {
	t.Helper()

	oldTier := viper.GetString("trading.paper.rate_limits.tier")
	oldEnabled := true
	if viper.IsSet("trading.paper.rate_limits.enabled") {
		oldEnabled = viper.GetBool("trading.paper.rate_limits.enabled")
	}

	viper.Set("trading.paper.rate_limits.enabled", true)
	viper.Set("trading.paper.rate_limits.tier", tier)

	t.Cleanup(func() {
		viper.Set("trading.paper.rate_limits.enabled", oldEnabled)
		viper.Set("trading.paper.rate_limits.tier", oldTier)
	})
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
