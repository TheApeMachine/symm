package level3

import (
	"testing"
	"time"
)

var fallback = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestParseSnapshotTreatsEntriesAsAdds(t *testing.T) {
	payload := []byte(`{
		"channel":"level3","type":"snapshot",
		"data":[{"symbol":"BTC/EUR",
			"bids":[{"order_id":"b1","limit_price":100.5,"order_qty":2.0,"timestamp":"2026-01-01T12:00:00.123456Z"}],
			"asks":[{"order_id":"a1","limit_price":101.0,"order_qty":3.0,"timestamp":"2026-01-01T12:00:00.123456Z"}]}]}`)

	orders, ok := ParseOrders(payload, fallback)

	if !ok || len(orders) != 2 {
		t.Fatalf("expected 2 orders, got %d ok=%v", len(orders), ok)
	}

	bid := orders[0]

	if bid.Event != "add" || bid.Side != SideBid || bid.OrderID != "b1" || bid.Price != 100.5 || bid.Qty != 2.0 {
		t.Fatalf("unexpected bid: %+v", bid)
	}

	if orders[1].Side != SideAsk || orders[1].Event != "add" {
		t.Fatalf("unexpected ask: %+v", orders[1])
	}

	if bid.Ts.Equal(fallback) {
		t.Fatal("expected the wire timestamp to be parsed, not the fallback")
	}
}

func TestParseUpdatePreservesPerEntryEvent(t *testing.T) {
	payload := []byte(`{
		"channel":"level3","type":"update",
		"data":[{"symbol":"BTC/EUR",
			"bids":[{"event":"delete","order_id":"b1","limit_price":100.5,"order_qty":2.0,"timestamp":"2026-01-01T12:00:01Z"}],
			"asks":[]}]}`)

	orders, ok := ParseOrders(payload, fallback)

	if !ok || len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d ok=%v", len(orders), ok)
	}

	if orders[0].Event != "delete" {
		t.Fatalf("expected delete event preserved, got %q", orders[0].Event)
	}
}

func TestParseNonLevel3FrameIsIgnored(t *testing.T) {
	if _, ok := ParseOrders([]byte(`{"channel":"heartbeat"}`), fallback); ok {
		t.Fatal("non-level3 frame must not parse as orders")
	}

	if _, ok := ParseOrders([]byte(`{"method":"subscribe","success":true}`), fallback); ok {
		t.Fatal("an ack frame must not parse as orders")
	}
}

func TestParseSkipsEntriesWithoutOrderID(t *testing.T) {
	payload := []byte(`{
		"channel":"level3","type":"update",
		"data":[{"symbol":"BTC/EUR",
			"bids":[{"event":"add","order_id":"","limit_price":1,"order_qty":1,"timestamp":""}],
			"asks":[]}]}`)

	if _, ok := ParseOrders(payload, fallback); ok {
		t.Fatal("an entry with no order_id must be skipped, leaving no orders")
	}
}

func TestParseFallsBackOnMissingTimestamp(t *testing.T) {
	payload := []byte(`{
		"channel":"level3","type":"update",
		"data":[{"symbol":"BTC/EUR",
			"bids":[{"event":"add","order_id":"b1","limit_price":1,"order_qty":1,"timestamp":""}],
			"asks":[]}]}`)

	orders, ok := ParseOrders(payload, fallback)

	if !ok || len(orders) != 1 {
		t.Fatalf("expected 1 order, got %d ok=%v", len(orders), ok)
	}

	if !orders[0].Ts.Equal(fallback) {
		t.Fatalf("expected fallback timestamp, got %v", orders[0].Ts)
	}
}
