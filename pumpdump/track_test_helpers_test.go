package pumpdump

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
)

func testTrackStore(t *testing.T) (*TrackStore, *qpool.Q) {
	t.Helper()

	ctx := context.Background()
	pool := qpool.NewQ(ctx, 1, 2, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	tick := pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
	trade := pool.CreateBroadcastGroup("trade", 10*time.Millisecond)
	book := pool.CreateBroadcastGroup("book", 10*time.Millisecond)

	trackStore, err := NewTrackStore(ctx, tick, trade, book, engine.DefaultCalibrationParams())

	if err != nil {
		t.Fatalf("new track store: %v", err)
	}

	return trackStore, pool
}

func publishTick(
	pool *qpool.Q,
	symbol string,
	last, volumeBase float64,
) {
	pool.CreateBroadcastGroup("tick", 10*time.Millisecond).Send(&qpool.QValue[any]{
		SenderID: "test",
		Value: engine.TickUpdate{
			Symbol:     symbol,
			Last:       last,
			VolumeBase: volumeBase,
		},
	})
}

func publishTrade(
	pool *qpool.Q,
	symbol string,
	volume float64,
	updated time.Time,
) {
	pool.CreateBroadcastGroup("trade", 10*time.Millisecond).Send(&qpool.QValue[any]{
		SenderID: "test",
		Value: engine.TradeUpdate{
			Symbol:      symbol,
			BatchVolume: volume,
			UpdatedAt:   updated,
		},
	})
}

func publishBook(pool *qpool.Q, symbol string, spreadBPS float64) {
	pool.CreateBroadcastGroup("book", 10*time.Millisecond).Send(&qpool.QValue[any]{
		SenderID: "test",
		Value: engine.BookUpdate{
			Symbol:    symbol,
			SpreadBPS: spreadBPS,
		},
	})
}

func drainTrack(trackStore *TrackStore) {
	for trackStore.Tick() {
	}
}
