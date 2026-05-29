package trader

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAudit(t *testing.T) {
	Convey("Given audit", t, func() {
		Convey("It should not panic for structured fields", func() {
			audit("test_event", map[string]any{
				"symbol": "BTC/EUR",
				"edge":   0.01,
			})
		})
	})
}

func TestAuditPublishesUIFrame(t *testing.T) {
	Convey("Given an audit UI broadcast", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		t.Cleanup(func() { pool.Close() })

		group := pool.CreateBroadcastGroup("audit-ui-test", 10*time.Millisecond)
		subscriber := group.Subscribe("test:audit-ui", 8)
		setAuditBroadcast(group)
		t.Cleanup(func() { clearAuditBroadcast(group) })

		audit("trade_entry_fill", map[string]any{
			"symbol":     "BTC/EUR",
			"slot_eur":   10,
			"confidence": 0.9,
		})

		Convey("It should publish a realtime trade action event", func() {
			select {
			case value := <-subscriber.Incoming:
				frame, ok := value.Value.(map[string]any)
				So(ok, ShouldBeTrue)
				So(frame["event"], ShouldEqual, "audit")
				So(frame["audit_event"], ShouldEqual, "trade_entry_fill")
				So(frame["symbol"], ShouldEqual, "BTC/EUR")
				So(frame["slot_eur"], ShouldEqual, 10)
				So(frame["seq"], ShouldNotBeNil)
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for audit UI frame")
			}
		})
	})
}

func TestAuditSuppressesNoisyUIFrames(t *testing.T) {
	Convey("Given an audit UI broadcast", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		t.Cleanup(func() { pool.Close() })

		group := pool.CreateBroadcastGroup("audit-ui-suppressed-test", 10*time.Millisecond)
		subscriber := group.Subscribe("test:audit-ui-suppressed", 8)
		setAuditBroadcast(group)
		t.Cleanup(func() { clearAuditBroadcast(group) })

		audit("measurement_ingest", map[string]any{
			"symbol":     "BTC/EUR",
			"source":     "cvd",
			"confidence": 0.91,
		})
		audit("trade_entry_skip", map[string]any{
			"symbol": "BTC/EUR",
			"reason": "edge_below_threshold",
			"edge":   0.01,
		})

		Convey("It should keep skips and high-volume telemetry out of the realtime audit panel", func() {
			select {
			case value := <-subscriber.Incoming:
				t.Fatalf("unexpected realtime audit frame: %+v", value.Value)
			case <-time.After(20 * time.Millisecond):
			}
		})
	})
}

func BenchmarkPublishAuditFrame(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	b.Cleanup(func() { pool.Close() })

	group := pool.CreateBroadcastGroup("audit-ui-bench", 10*time.Millisecond)
	_ = group.Subscribe("bench:audit-ui", 1024)
	setAuditBroadcast(group)
	b.Cleanup(func() { clearAuditBroadcast(group) })

	fields := map[string]any{
		"event":      "trade_entry_fill",
		"ts":         time.Now().UTC().Format(time.RFC3339Nano),
		"symbol":     "BTC/EUR",
		"slot_eur":   10,
		"confidence": 0.9,
	}

	b.ReportAllocs()

	for b.Loop() {
		publishAuditFrame("trade_entry_fill", fields)
	}
}
