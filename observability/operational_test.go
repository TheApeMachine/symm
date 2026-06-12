package observability

import (
	"errors"
	"testing"
	"time"
)

func TestOperationalMetricsSnapshot(t *testing.T) {
	metrics := NewOperationalMetrics()
	observedAt := time.Date(2026, 6, 12, 20, 0, 0, 0, time.UTC)

	metrics.RecordBusSend("raw", "ticker", observedAt)
	metrics.RecordBusReceive("raw", "ticker", observedAt.Add(5*time.Millisecond))
	metrics.RecordBusDrop("raw", "book", "expired message", observedAt)
	metrics.RecordWebSocketReconnect("kraken/public", "wss://example", "boom", observedAt)
	metrics.RecordWebSocketConnected("kraken/public", "wss://example", observedAt)
	metrics.RecordExchangeError(
		"kraken/private",
		"EOrder",
		"Cannot open position",
		"reject_order",
		"Cannot open position",
		observedAt,
	)
	metrics.RecordMarketDataAge(
		"ticker",
		"BTC/USD",
		observedAt.Add(-time.Second),
		observedAt,
	)
	metrics.RecordOrderSubmitted(OrderCorrelation{
		DecisionID: "decision-1",
		ActionID:   "action-1",
		ClOrdID:    "cl-1",
		Symbol:     "BTC/USD",
	}, 3*time.Millisecond, 10, observedAt)
	metrics.RecordOrderExecution(
		"cl-1",
		"order-1",
		"",
		"new",
		"",
		observedAt.Add(4*time.Millisecond),
	)
	metrics.RecordOrderExecution(
		"cl-1",
		"order-1",
		"exec-1",
		"filled",
		"trade",
		observedAt.Add(9*time.Millisecond),
	)
	metrics.RecordRiskReject("ETH/USD", "quote stale", observedAt)
	metrics.RecordStopTriggered("BTC/USD", observedAt)
	metrics.RecordStopExitSubmitted(
		"BTC/USD",
		observedAt,
		observedAt.Add(2*time.Millisecond),
	)
	metrics.RecordStopExitFilled(
		"BTC/USD",
		observedAt,
		observedAt.Add(8*time.Millisecond),
	)
	metrics.RecordStopNeedsRepair("BTC/USD", "send failed", observedAt)
	metrics.RecordExposure("USD", 1, 100, 10, 5, observedAt)
	metrics.RecordAuditWriteFailure(errors.New("disk full"), observedAt)

	snapshot := metrics.Snapshot()

	if len(snapshot.Bus) != 2 {
		t.Fatalf("expected 2 bus snapshots, got %d", len(snapshot.Bus))
	}

	if snapshot.Bus[1].Sent != 1 || snapshot.Bus[1].Received != 1 {
		t.Fatalf("unexpected raw ticker bus counts: %+v", snapshot.Bus[1])
	}

	if snapshot.WebSockets[0].Reconnects != 1 {
		t.Fatalf("expected reconnect count, got %+v", snapshot.WebSockets[0])
	}

	if snapshot.MarketData[0].Age != time.Second {
		t.Fatalf("expected ticker age, got %+v", snapshot.MarketData[0])
	}

	if snapshot.ExchangeErrors[0].Action != "reject_order" {
		t.Fatalf("unexpected exchange error snapshot: %+v", snapshot.ExchangeErrors)
	}

	if snapshot.Orders.Submitted != 1 || snapshot.Orders.Filled != 1 {
		t.Fatalf("unexpected order counts: %+v", snapshot.Orders)
	}

	if snapshot.Orders.Acknowledged != 1 || snapshot.Orders.Rejected != 1 {
		t.Fatalf("unexpected order status counts: %+v", snapshot.Orders)
	}

	if len(snapshot.Orders.Correlations) != 1 {
		t.Fatalf("expected one correlation, got %+v", snapshot.Orders.Correlations)
	}

	if snapshot.Stops.Triggered != 1 || snapshot.Stops.ExitFilled != 1 {
		t.Fatalf("unexpected stop counts: %+v", snapshot.Stops)
	}

	if snapshot.Exposure.OpenExposure != 100 {
		t.Fatalf("unexpected exposure snapshot: %+v", snapshot.Exposure)
	}

	if snapshot.Audit.WriteFailures != 1 {
		t.Fatalf("unexpected audit snapshot: %+v", snapshot.Audit)
	}
}

func BenchmarkOperationalMetricsOrderLifecycle(benchmark *testing.B) {
	metrics := NewOperationalMetrics()
	observedAt := time.Date(2026, 6, 12, 20, 0, 0, 0, time.UTC)
	correlation := OrderCorrelation{
		DecisionID: "decision-1",
		ActionID:   "action-1",
		ClOrdID:    "cl-1",
		Symbol:     "BTC/USD",
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		metrics.RecordOrderSubmitted(
			correlation,
			time.Microsecond,
			10,
			observedAt,
		)
		metrics.RecordOrderExecution(
			correlation.ClOrdID,
			"order-1",
			"exec-1",
			"filled",
			"trade",
			observedAt.Add(time.Millisecond),
		)
	}
}
