package internal

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/observability"
)

func TestBusRecordsOperationalMetrics(test *testing.T) {
	observability.ResetSharedForTest()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 4, nil)
	defer pool.Close()

	bus := NewBus(
		ctx,
		pool,
		[]Channel{ChannelRaw},
		[]Subscription{Subscribe(ChannelRaw, "bus-metrics-test")},
	)
	defer func() {
		_ = bus.Close()
	}()

	if err := bus.Send(ChannelRaw, "ticker", "payload"); err != nil {
		test.Fatalf("send: %v", err)
	}

	if _, err := bus.Receive(ChannelRaw); err != nil {
		test.Fatalf("receive: %v", err)
	}

	snapshot := observability.Shared().Snapshot()

	if len(snapshot.Bus) != 1 {
		test.Fatalf("expected one bus snapshot, got %+v", snapshot.Bus)
	}

	channel := snapshot.Bus[0]

	if channel.Sent != 1 || channel.Received != 1 {
		test.Fatalf("unexpected bus counts: %+v", channel)
	}

	if channel.Outstanding != 0 {
		test.Fatalf("expected no outstanding messages, got %+v", channel)
	}
}
