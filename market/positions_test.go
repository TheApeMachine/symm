package market

import (
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

/*
TestPositionReadingsDerivesPnl proves position economics are derived on the
backend from the tree alone: a balances frame for an open asset, an entry fill in
executions, and a ticker mark combine into one reading with the correct
unrealized P&L and percent change — nothing the frontend has to recompute.
*/
func TestPositionReadingsDerivesPnl(t *testing.T) {
	tree := dmt.NewTree("")

	balances := datura.Acquire("paper", datura.APPJSON).
		WithRole("balances").
		WithScope("balances").
		WithPayload(datura.Map[any]{
			"asset": []any{
				datura.Map[any]{"asset": "USD", "balance": 1000.0},
				datura.Map[any]{"asset": "XBT", "balance": 2.0},
			},
		}.Marshal())
	tree.InsertArtifact(balances.Prefix("role", "timestamp"), balances)

	fill := datura.Acquire("paper", datura.APPJSON).
		WithRole("executions").
		WithScope("executions").
		WithPayload(datura.Map[any]{
			"data": []any{
				datura.Map[any]{"symbol": "XBT/USD", "side": "buy", "avg_price": 100.0},
			},
		}.Marshal())
	tree.InsertArtifact(fill.Prefix("role", "timestamp"), fill)

	ticker := datura.Acquire("websocket", datura.APPJSON).
		WithRole("ticker").
		WithScope("XBT/USD").
		WithPayload(datura.Map[any]{
			"channel": "ticker",
			"data": []any{
				datura.Map[any]{"symbol": "XBT/USD", "last": 150.0, "volume": 5.0},
			},
		}.Marshal())
	tree.InsertArtifact(ticker.Prefix("role", "timestamp"), ticker)

	stop := datura.Acquire("broker", datura.APPJSON).
		WithRole("stoploss").
		WithScope("XBT/USD").
		WithPayload(datura.Map[any]{
			"stop":   140.0,
			"peak":   155.0,
			"offset": 0.1,
			"side":   "sell",
		}.Marshal())
	tree.InsertArtifact(stop.Prefix("role", "scope"), stop)

	readings := PositionReadings(tree, "USD")

	if len(readings) != 1 {
		t.Fatalf("expected one open position, got %d", len(readings))
	}

	reading := readings[0]

	if reading["symbol"] != "XBT/USD" {
		t.Fatalf("expected XBT/USD, got %q", reading["symbol"])
	}

	if reading["entry"] != 100.0 || reading["mark"] != 150.0 {
		t.Fatalf("expected entry 100 mark 150, got entry %.2f mark %.2f", reading["entry"], reading["mark"])
	}

	// (150 - 100) * 2 = 100 unrealized; (150-100)/100 = 50%.
	if reading["unrealizedPnl"] != 100.0 {
		t.Fatalf("expected unrealized P&L 100, got %.2f", reading["unrealizedPnl"])
	}

	if reading["changePct"] != 50.0 {
		t.Fatalf("expected change 50%%, got %.2f", reading["changePct"])
	}

	if reading["stop"] != 140.0 || reading["peak"] != 155.0 || reading["offset"] != 0.1 {
		t.Fatalf("expected stop state, got %v", reading)
	}
}

/*
TestPositionReadingsNoLedger proves "no ledger" stays distinct from "flat": with
no balances frame published, the reader returns nil rather than an empty slice
that would read as a confirmed-empty portfolio.
*/
func TestPositionReadingsNoLedger(t *testing.T) {
	tree := dmt.NewTree("")

	if readings := PositionReadings(tree, "USD"); readings != nil {
		t.Fatalf("expected nil with no balances, got %v", readings)
	}
}

func TestPositionReadingsFlatLedgerReturnsEmpty(t *testing.T) {
	tree := dmt.NewTree("")

	balances := datura.Acquire("paper", datura.APPJSON).
		WithRole("balances").
		WithScope("balances").
		WithPayload(datura.Map[any]{
			"asset": []any{
				datura.Map[any]{"asset": "USD", "balance": 1000.0},
			},
		}.Marshal())
	tree.InsertArtifact(balances.Prefix("role", "timestamp"), balances)

	if readings := PositionReadings(tree, "USD"); len(readings) != 0 {
		t.Fatalf("expected empty flat-ledger readings, got %v", readings)
	}
}
