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

	readings, readErr := PositionReadings(tree, "USD")
	if readErr != nil {
		t.Fatal(readErr)
	}

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

	readings, readErr := PositionReadings(tree, "USD")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if readings != nil {
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

	readings, readErr := PositionReadings(tree, "USD")
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(readings) != 0 {
		t.Fatalf("expected empty flat-ledger readings, got %v", readings)
	}
}

func TestPositionReadingsReadsWrappedExecutionMap(t *testing.T) {
	tree := dmt.NewTree("")

	balances := datura.Acquire("paper", datura.APPJSON).
		WithRole("balances").
		WithScope("balances").
		WithPayload(datura.Map[any]{
			"asset": []any{
				datura.Map[any]{"asset": "USD", "balance": 1000.0},
				datura.Map[any]{"asset": "ALGO", "balance": 10.0},
			},
		}.Marshal())
	tree.InsertArtifact(balances.Prefix("role", "timestamp"), balances)

	fill := datura.Acquire("paper", datura.APPJSON).
		WithRole("executions").
		WithScope("update").
		WithPayload(datura.Map[any]{
			"executions": datura.Map[any]{
				"exec-1": datura.Map[any]{
					"symbol":     "ALGO/USD",
					"side":       "buy",
					"last_price": 0.10,
					"avg_price":  0.10,
				},
			},
		}.Marshal())
	tree.InsertArtifact(fill.Prefix("role", "timestamp"), fill)

	ticker := datura.Acquire("websocket", datura.APPJSON).
		WithRole("ticker").
		WithScope("ALGO/USD").
		WithPayload(datura.Map[any]{
			"channel": "ticker",
			"data": []any{
				datura.Map[any]{"symbol": "ALGO/USD", "last": 0.12, "volume": 5.0},
			},
		}.Marshal())
	tree.InsertArtifact(ticker.Prefix("role", "timestamp"), ticker)

	readings, readErr := PositionReadings(tree, "USD")
	if readErr != nil {
		t.Fatal(readErr)
	}

	if len(readings) != 1 {
		t.Fatalf("expected one open position, got %d", len(readings))
	}

	if readings[0]["entry"] != 0.10 || readings[0]["mark"] != 0.12 {
		t.Fatalf("expected wrapped fill entry and ticker mark, got %v", readings[0])
	}
}

func TestPositionReadingsErrorsWithoutEntryFill(t *testing.T) {
	tree := dmt.NewTree("")

	balances := datura.Acquire("paper", datura.APPJSON).
		WithRole("balances").
		WithScope("balances").
		WithPayload(datura.Map[any]{
			"asset": []any{
				datura.Map[any]{"asset": "USD", "balance": 1000.0},
				datura.Map[any]{"asset": "ALGO", "balance": 10.0},
			},
		}.Marshal())
	tree.InsertArtifact(balances.Prefix("role", "timestamp"), balances)

	ticker := datura.Acquire("websocket", datura.APPJSON).
		WithRole("ticker").
		WithScope("ALGO/USD").
		WithPayload(datura.Map[any]{
			"channel": "ticker",
			"data": []any{
				datura.Map[any]{"symbol": "ALGO/USD", "last": 0.12, "volume": 5.0},
			},
		}.Marshal())
	tree.InsertArtifact(ticker.Prefix("role", "timestamp"), ticker)

	if _, readErr := PositionReadings(tree, "USD"); readErr == nil {
		t.Fatal("expected missing entry fill error")
	}
}
