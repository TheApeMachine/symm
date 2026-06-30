package broker

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func testDesk(t *testing.T) (*Desk, <-chan *datura.Artifact) {
	t.Helper()

	oldQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() {
		viper.Set("market.quote_currency", oldQuote)
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	pool := qpool.NewQ[any](ctx, 4, 8, nil)
	desk := NewDesk(ctx, pool, dmt.NewTree(""))
	t.Cleanup(func() {
		_ = desk.Close()
	})

	orders := make(chan *datura.Artifact, 8)
	pool.Subscribe("kraken:private", func(artifact *datura.Artifact) error {
		orders <- artifact
		return nil
	})

	return desk, orders
}

func balancesFrame(usd float64, base string, baseQty float64) *datura.Artifact {
	return datura.Acquire("test", datura.APPJSON).
		WithRole("balances").
		WithScope("balances").
		WithPayload(datura.Map[any]{
			"channel": "balances",
			"type":    "snapshot",
			"data": []datura.Map[any]{
				{"asset": "USD", "balance": usd},
				{"asset": base, "balance": baseQty},
			},
		}.Marshal())
}

func legacyBalancesFrame(usd float64, base string, baseQty float64) *datura.Artifact {
	return datura.Acquire("test", datura.APPJSON).
		WithRole("balances").
		WithScope("balances").
		WithPayload(datura.Map[any]{
			"asset": []datura.Map[any]{
				{"asset": "USD", "balance": usd},
				{"asset": base, "balance": baseQty},
			},
		}.Marshal())
}

func tickerFrame(symbol string, last float64) *datura.Artifact {
	return datura.Acquire("test", datura.APPJSON).
		WithRole("ticker").
		WithScope("ticker").
		WithPayload(datura.Map[any]{
			"channel": "ticker",
			"type":    "update",
			"data": []datura.Map[any]{
				{"symbol": symbol, "last": last},
			},
		}.Marshal())
}

func tickerQuoteFrame(symbol string, bid, ask, last float64) *datura.Artifact {
	return datura.Acquire("test", datura.APPJSON).
		WithRole("ticker").
		WithScope("ticker").
		WithPayload(datura.Map[any]{
			"channel": "ticker",
			"type":    "update",
			"data": []datura.Map[any]{
				{"symbol": symbol, "bid": bid, "ask": ask, "last": last},
			},
		}.Marshal())
}

func actionFrame(symbol, actionType, side string, fraction float64) *datura.Artifact {
	return datura.Acquire("story", datura.APPJSON).
		WithRole(side).
		WithScope(symbol).
		WithPayload(datura.Map[any]{
			"type":     actionType,
			"side":     side,
			"fraction": fraction,
		}.Marshal()).
		Poke(true, "allowed").
		Poke(true, "risk", "stamped")
}

func receiveOrder(t *testing.T, orders <-chan *datura.Artifact) *datura.Artifact {
	t.Helper()

	select {
	case order := <-orders:
		return order
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for order")
		return nil
	}
}

func assertNoOrder(t *testing.T, orders <-chan *datura.Artifact) {
	t.Helper()

	select {
	case order := <-orders:
		t.Fatalf("unexpected order: %v", order)
	case <-time.After(100 * time.Millisecond):
	}
}

func subscribeDiagnostics(pool *qpool.Q[any]) <-chan *datura.Artifact {
	diagnostics := make(chan *datura.Artifact, 8)
	pool.Subscribe("ui", func(artifact *datura.Artifact) error {
		if datura.Peek[string](artifact, "role") == "diagnostic" {
			diagnostics <- artifact
		}
		return nil
	})
	return diagnostics
}

func receiveDiagnostic(t *testing.T, diagnostics <-chan *datura.Artifact) *datura.Artifact {
	t.Helper()

	select {
	case diagnostic := <-diagnostics:
		return diagnostic
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for diagnostic")
		return nil
	}
}

func TestDeskSizesBuyFromQuoteBalanceAndMark(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 0.5)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "market", "buy", 0.05)
	t.Cleanup(action.Release)

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	if qty := datura.Peek[float64](order, "params", "order_qty"); qty != 20.0 {
		t.Fatalf("order_qty = %v, want 20", qty)
	}
	if side := datura.Peek[string](order, "params", "side"); side != "buy" {
		t.Fatalf("side = %q, want buy", side)
	}

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}
	assertNoOrder(t, orders)
}

func TestDeskSubmitsLimitOrderWithLivePassivePrice(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerQuoteFrame("MATIC/USD", 0.49, 0.51, 0.50)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "limit", "buy", 0.05)
	t.Cleanup(action.Release)

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	if orderType := datura.Peek[string](order, "params", "order_type"); orderType != "limit" {
		t.Fatalf("order_type = %q, want limit", orderType)
	}
	if limitPrice := datura.Peek[float64](order, "params", "limit_price"); limitPrice != 0.49 {
		t.Fatalf("limit_price = %v, want 0.49", limitPrice)
	}
	if qty := datura.Peek[float64](order, "params", "order_qty"); math.Abs(qty-(200.0*0.05/0.51)) > 1e-12 {
		t.Fatalf("order_qty = %v, want ask-priced quantity", qty)
	}
}

func TestDeskStampsSetupKeyOnOrderPayload(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 0.5)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "market", "buy", 0.05).
		Poke("fluid", "reason_source").
		Poke("laminar", "reason_category")
	t.Cleanup(action.Release)

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	if setupKey := datura.Peek[string](order, "params", "setup_key"); setupKey != "fluid|laminar|buy|market" {
		t.Fatalf("setup_key = %q, want fluid|laminar|buy|market", setupKey)
	}
}

func TestDeskClearsPendingBuyAfterRejectedExecution(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 0.5)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "market", "buy", 0.05)
	t.Cleanup(action.Release)

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	orderID := datura.Peek[string](order, "params", "cl_ord_id")
	if orderID == "" {
		t.Fatal("order missing cl_ord_id")
	}

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}
	assertNoOrder(t, orders)

	rejected := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithPayload(datura.Map[any]{
			"order_status": "rejected",
			"cl_ord_id":    orderID,
		}.Marshal())
	t.Cleanup(rejected.Release)
	if err := desk.onMessage(rejected); err != nil {
		t.Fatal(err)
	}

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	receiveOrder(t, orders)
}

func TestDeskUnmatchedTerminalExecutionDoesNotClearPending(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 0.5)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "market", "buy", 0.05)
	t.Cleanup(action.Release)
	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	orderID := datura.Peek[string](order, "params", "cl_ord_id")
	if orderID == "" {
		t.Fatal("order missing cl_ord_id")
	}

	unmatched := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithPayload(datura.Map[any]{
			"order_status": "rejected",
			"cl_ord_id":    "not-" + orderID,
			"symbol":       "MATIC/USD",
			"side":         "buy",
		}.Marshal())
	t.Cleanup(unmatched.Release)
	if err := desk.onMessage(unmatched); err != nil {
		t.Fatal(err)
	}

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}
	assertNoOrder(t, orders)

	matched := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithPayload(datura.Map[any]{
			"order_status": "rejected",
			"cl_ord_id":    orderID,
			"symbol":       "MATIC/USD",
			"side":         "buy",
		}.Marshal())
	t.Cleanup(matched.Release)
	if err := desk.onMessage(matched); err != nil {
		t.Fatal(err)
	}

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}
	receiveOrder(t, orders)
}

func TestPendingTerminalDoesNotClearUnrelatedSameSideOrder(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 0.5)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "market", "buy", 0.05)
	t.Cleanup(action.Release)
	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	firstOrder := receiveOrder(t, orders)
	firstID := datura.Peek[string](firstOrder, "params", "cl_ord_id")
	if firstID == "" {
		t.Fatal("first order missing cl_ord_id")
	}

	ack := datura.Acquire("test", datura.APPJSON).
		WithRole("orders").
		WithPayload(datura.Map[any]{
			"order_status": "open",
			"cl_ord_id":    firstID,
			"order_id":     "EX-1",
			"symbol":       "MATIC/USD",
			"side":         "buy",
		}.Marshal())
	t.Cleanup(ack.Release)
	if err := desk.onMessage(ack); err != nil {
		t.Fatal(err)
	}

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}
	secondOrder := receiveOrder(t, orders)
	secondID := datura.Peek[string](secondOrder, "params", "cl_ord_id")
	if secondID == "" || secondID == firstID {
		t.Fatalf("second cl_ord_id = %q, first = %q", secondID, firstID)
	}

	rejectedFirst := datura.Acquire("test", datura.APPJSON).
		WithRole("orders").
		WithPayload(datura.Map[any]{
			"order_status": "rejected",
			"cl_ord_id":    firstID,
			"order_id":     "EX-1",
			"symbol":       "MATIC/USD",
			"side":         "buy",
		}.Marshal())
	t.Cleanup(rejectedFirst.Release)
	if err := desk.onMessage(rejectedFirst); err != nil {
		t.Fatal(err)
	}

	if _, ok := desk.pendingByClOrdID.Load(firstID); ok {
		t.Fatal("first order still pending after terminal update")
	}
	if _, ok := desk.pendingByClOrdID.Load(secondID); !ok {
		t.Fatal("second same-side order was cleared by first terminal update")
	}
	if count := desk.pendingCount(); count != 1 {
		t.Fatalf("pendingCount = %d, want 1", count)
	}
}

func TestExchangeOnlyTerminalRequiresPriorAckBinding(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 0.5)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "market", "buy", 0.05)
	t.Cleanup(action.Release)
	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	orderID := datura.Peek[string](order, "params", "cl_ord_id")
	if orderID == "" {
		t.Fatal("order missing cl_ord_id")
	}

	unknownExchangeOnly := datura.Acquire("test", datura.APPJSON).
		WithRole("orders").
		WithPayload(datura.Map[any]{
			"order_status": "rejected",
			"order_id":     "EX-unknown",
			"symbol":       "MATIC/USD",
			"side":         "buy",
		}.Marshal())
	t.Cleanup(unknownExchangeOnly.Release)
	if err := desk.onMessage(unknownExchangeOnly); err != nil {
		t.Fatal(err)
	}
	if _, ok := desk.pendingByClOrdID.Load(orderID); !ok {
		t.Fatal("exchange-only terminal update cleared unbound client order")
	}

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}
	assertNoOrder(t, orders)

	ack := datura.Acquire("test", datura.APPJSON).
		WithRole("orders").
		WithPayload(datura.Map[any]{
			"order_status": "open",
			"cl_ord_id":    orderID,
			"order_id":     "EX-bound",
			"symbol":       "MATIC/USD",
			"side":         "buy",
		}.Marshal())
	t.Cleanup(ack.Release)
	if err := desk.onMessage(ack); err != nil {
		t.Fatal(err)
	}

	boundExchangeOnly := datura.Acquire("test", datura.APPJSON).
		WithRole("orders").
		WithPayload(datura.Map[any]{
			"order_status": "rejected",
			"order_id":     "EX-bound",
			"symbol":       "MATIC/USD",
			"side":         "buy",
		}.Marshal())
	t.Cleanup(boundExchangeOnly.Release)
	if err := desk.onMessage(boundExchangeOnly); err != nil {
		t.Fatal(err)
	}

	if _, ok := desk.pendingByClOrdID.Load(orderID); ok {
		t.Fatal("bound exchange-id terminal update did not clear client order")
	}
}

func TestDeskPendingOrderAckTimeoutEmitsDiagnosticAndBlocksDuplicate(t *testing.T) {
	oldTimeout := viper.GetDuration("trading.order_ack_timeout")
	viper.Set("trading.order_ack_timeout", time.Nanosecond)
	t.Cleanup(func() {
		viper.Set("trading.order_ack_timeout", oldTimeout)
	})

	desk, orders := testDesk(t)
	diagnostics := subscribeDiagnostics(desk.pool)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 0.5)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "market", "buy", 0.05)
	t.Cleanup(action.Release)
	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}
	receiveOrder(t, orders)

	time.Sleep(time.Millisecond)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	diagnostic := receiveDiagnostic(t, diagnostics)
	if reason := datura.Peek[string](diagnostic, "reason"); reason != "order_ack_timeout" {
		t.Fatalf("diagnostic reason = %q, want order_ack_timeout", reason)
	}
	if symbol := datura.Peek[string](diagnostic, "symbol"); symbol != "MATIC/USD" {
		t.Fatalf("diagnostic symbol = %q, want MATIC/USD", symbol)
	}

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}
	assertNoOrder(t, orders)
}

func TestDeskRefusesBlockedDecisionWithoutRiskReason(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 0.5)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "market", "buy", 0.05).
		WithAttribute("verdict", "blocked").
		WithAttribute("why", "edge_unavailable")
	t.Cleanup(action.Release)

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	assertNoOrder(t, orders)
}

func TestDeskRefusesBuyWithoutAllocatorStamp(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 0.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 0.5)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	action := datura.Acquire("story", datura.APPJSON).
		WithRole("buy").
		WithScope("MATIC/USD").
		WithPayload(datura.Map[any]{
			"type":     "market",
			"side":     "buy",
			"fraction": 0.05,
		}.Marshal())
	t.Cleanup(action.Release)

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	assertNoOrder(t, orders)
}

func TestDeskProtectiveActionDoesNotSerializeAsMarket(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 10.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "stop_loss", "sell", 0).
		Poke(0.45, "trigger_price")
	t.Cleanup(action.Release)

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	if orderType := datura.Peek[string](order, "params", "order_type"); orderType != "stop-loss" {
		t.Fatalf("order_type = %q, want stop-loss", orderType)
	}
	if trigger := datura.Peek[float64](order, "params", "trigger_price"); trigger != 0.45 {
		t.Fatalf("trigger_price = %v, want 0.45", trigger)
	}
}

func TestDeskLiveBuyFillSubmitsNativeProtectiveStop(t *testing.T) {
	oldModel := viper.GetString("trading.model")
	oldNative := viper.GetBool("live.native_protective_stops_supported")
	oldOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
	viper.Set("trading.model", "live")
	viper.Set("live.native_protective_stops_supported", true)
	viper.Set("trading.stop.trailing_offset_bps", 100.0)
	t.Setenv("SYMM_LIVE", "true")
	t.Cleanup(func() {
		viper.Set("trading.model", oldModel)
		viper.Set("live.native_protective_stops_supported", oldNative)
		viper.Set("trading.stop.trailing_offset_bps", oldOffset)
	})

	desk, orders := testDesk(t)

	fill := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithScope("MATIC/USD").
		Poke("filled", "order_status").
		Poke("buy", "side").
		Poke(100.0, "last_price").
		Poke(10.0, "order_qty")
	t.Cleanup(fill.Release)
	if err := desk.onMessage(fill); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	if side := datura.Peek[string](order, "params", "side"); side != "sell" {
		t.Fatalf("side = %q, want sell", side)
	}
	if orderType := datura.Peek[string](order, "params", "order_type"); orderType != "trailing-stop" {
		t.Fatalf("order_type = %q, want trailing-stop", orderType)
	}
	if qty := datura.Peek[float64](order, "params", "order_qty"); qty != 10 {
		t.Fatalf("order_qty = %v, want 10", qty)
	}
	if trailing := datura.Peek[float64](order, "params", "trailing_stop"); trailing != 100.0 {
		t.Fatalf("trailing_stop = %v, want 100", trailing)
	}
	if !desk.nativeProtectionMissing("MATIC/USD") {
		t.Fatal("position should remain unprotected until protective ack")
	}

	protectiveID := datura.Peek[string](order, "params", "cl_ord_id")
	if protectiveID == "" {
		t.Fatal("protective order missing cl_ord_id")
	}
	ack := datura.Acquire("test", datura.APPJSON).
		WithRole("orders").
		WithPayload(datura.Map[any]{
			"order_status": "open",
			"cl_ord_id":    protectiveID,
			"order_id":     "STOP-1",
			"symbol":       "MATIC/USD",
			"side":         "sell",
		}.Marshal())
	t.Cleanup(ack.Release)
	if err := desk.onMessage(ack); err != nil {
		t.Fatal(err)
	}
	if desk.nativeProtectionMissing("MATIC/USD") {
		t.Fatal("position should be protected after protective ack")
	}

	ticker := tickerFrame("MATIC/USD", 98.0)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}
	assertNoOrder(t, orders)
}

func TestDeskSizesSellFromHeldBaseBalance(t *testing.T) {
	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 10.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "settle_position", "sell", 0)
	t.Cleanup(action.Release)

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	if qty := datura.Peek[float64](order, "params", "order_qty"); qty != 10.0 {
		t.Fatalf("order_qty = %v, want 10", qty)
	}
	if side := datura.Peek[string](order, "params", "side"); side != "sell" {
		t.Fatalf("side = %q, want sell", side)
	}
}

func TestDeskSizesSellFromLegacyAssetBalanceRows(t *testing.T) {
	desk, orders := testDesk(t)

	balances := legacyBalancesFrame(200.0, "MATIC", 10.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	action := actionFrame("MATIC/USD", "settle_position", "sell", 0)
	t.Cleanup(action.Release)

	if err := desk.Update([]*datura.Artifact{action}); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	if qty := datura.Peek[float64](order, "params", "order_qty"); qty != 10.0 {
		t.Fatalf("order_qty = %v, want 10", qty)
	}
}

func TestDeskSubmitsStopExitWhenTrailingStopBreaks(t *testing.T) {
	oldOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
	viper.Set("trading.stop.trailing_offset_bps", 100.0)
	t.Cleanup(func() {
		viper.Set("trading.stop.trailing_offset_bps", oldOffset)
	})

	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 10.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	fill := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithScope("MATIC/USD").
		Poke("filled", "order_status").
		Poke("buy", "side").
		Poke(100.0, "last_price")
	t.Cleanup(fill.Release)
	if err := desk.onMessage(fill); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 98.0)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	if qty := datura.Peek[float64](order, "params", "order_qty"); qty != 10.0 {
		t.Fatalf("order_qty = %v, want 10", qty)
	}
	if side := datura.Peek[string](order, "params", "side"); side != "sell" {
		t.Fatalf("side = %q, want sell", side)
	}
	if orderType := datura.Peek[string](order, "params", "order_type"); orderType != "market" {
		t.Fatalf("order_type = %q, want market", orderType)
	}
}

func TestDeskRetriesStopExitWhenBalanceArrives(t *testing.T) {
	oldOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
	viper.Set("trading.stop.trailing_offset_bps", 100.0)
	t.Cleanup(func() {
		viper.Set("trading.stop.trailing_offset_bps", oldOffset)
	})

	desk, orders := testDesk(t)

	fill := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithScope("MATIC/USD").
		Poke("filled", "order_status").
		Poke("buy", "side").
		Poke(100.0, "last_price")
	t.Cleanup(fill.Release)
	if err := desk.onMessage(fill); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 98.0)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	assertNoOrder(t, orders)

	balances := balancesFrame(200.0, "MATIC", 10.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	order := receiveOrder(t, orders)
	if qty := datura.Peek[float64](order, "params", "order_qty"); qty != 10.0 {
		t.Fatalf("order_qty = %v, want 10", qty)
	}
	if side := datura.Peek[string](order, "params", "side"); side != "sell" {
		t.Fatalf("side = %q, want sell", side)
	}
}

func TestDeskRetriesRejectedStopExitOnNextBalance(t *testing.T) {
	oldOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
	viper.Set("trading.stop.trailing_offset_bps", 100.0)
	t.Cleanup(func() {
		viper.Set("trading.stop.trailing_offset_bps", oldOffset)
	})

	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 10.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	fill := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithScope("MATIC/USD").
		Poke("filled", "order_status").
		Poke("buy", "side").
		Poke(100.0, "last_price")
	t.Cleanup(fill.Release)
	if err := desk.onMessage(fill); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 98.0)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	exitOrder := receiveOrder(t, orders)
	exitID := datura.Peek[string](exitOrder, "params", "cl_ord_id")
	if exitID == "" {
		t.Fatal("stop exit order missing cl_ord_id")
	}

	rejected := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithScope("MATIC/USD").
		Poke("rejected", "order_status").
		Poke("sell", "side").
		Poke(exitID, "cl_ord_id")
	t.Cleanup(rejected.Release)
	if err := desk.onMessage(rejected); err != nil {
		t.Fatal(err)
	}

	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	retry := receiveOrder(t, orders)
	if retryID := datura.Peek[string](retry, "params", "cl_ord_id"); retryID == "" || retryID == exitID {
		t.Fatalf("retry cl_ord_id = %q, want new order", retryID)
	}
}

func TestDeskRetriesCanceledStopExitOnNextTicker(t *testing.T) {
	oldOffset := viper.GetFloat64("trading.stop.trailing_offset_bps")
	viper.Set("trading.stop.trailing_offset_bps", 100.0)
	t.Cleanup(func() {
		viper.Set("trading.stop.trailing_offset_bps", oldOffset)
	})

	desk, orders := testDesk(t)

	balances := balancesFrame(200.0, "MATIC", 10.0)
	t.Cleanup(balances.Release)
	if err := desk.onMessage(balances); err != nil {
		t.Fatal(err)
	}

	fill := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithScope("MATIC/USD").
		Poke("filled", "order_status").
		Poke("buy", "side").
		Poke(100.0, "last_price")
	t.Cleanup(fill.Release)
	if err := desk.onMessage(fill); err != nil {
		t.Fatal(err)
	}

	ticker := tickerFrame("MATIC/USD", 98.0)
	t.Cleanup(ticker.Release)
	if err := desk.onMessage(ticker); err != nil {
		t.Fatal(err)
	}

	exitOrder := receiveOrder(t, orders)
	exitID := datura.Peek[string](exitOrder, "params", "cl_ord_id")
	if exitID == "" {
		t.Fatal("stop exit order missing cl_ord_id")
	}

	canceled := datura.Acquire("test", datura.APPJSON).
		WithRole("executions").
		WithScope("MATIC/USD").
		Poke("canceled", "order_status").
		Poke("sell", "side").
		Poke(exitID, "cl_ord_id")
	t.Cleanup(canceled.Release)
	if err := desk.onMessage(canceled); err != nil {
		t.Fatal(err)
	}

	if err := desk.onMessage(tickerFrame("MATIC/USD", 97.5)); err != nil {
		t.Fatal(err)
	}

	retry := receiveOrder(t, orders)
	if retryID := datura.Peek[string](retry, "params", "cl_ord_id"); retryID == "" || retryID == exitID {
		t.Fatalf("retry cl_ord_id = %q, want new order", retryID)
	}
}
