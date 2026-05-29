package hawkes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trade"
)

func storeHawkesSymbol(hawkes *Hawkes, symbol string, state *symbolState) {
	hawkes.symbols.Store(symbol, state)
}

func loadHawkesSymbol(hawkes *Hawkes, symbol string) *symbolState {
	raw, ok := hawkes.symbols.Load(symbol)

	if !ok {
		return nil
	}

	return raw.(*symbolState)
}

func startHawkesTick(t *testing.T, hawkes *Hawkes) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		defer close(done)

		if err := hawkes.Tick(); err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("hawkes tick: %v", err)
		}
	}()

	t.Cleanup(func() {
		_ = hawkes.Close()

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for hawkes tick to close")
		}
	})
}

func TestHawkesTick(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewHawkes(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	storeHawkesSymbol(signal, "BTC/EUR", &symbolState{
		pair:      asset.Pair{Wsname: "BTC/EUR", Quote: "EUR"},
		state:     NewHawkesSymbol(engine.DefaultCalibrationParams()),
		ticks:     make([]trade.Data, 0, 32),
		imbalance: 0,
	})

	startHawkesTick(t, signal)

	pool.CreateBroadcastGroup("book", 0).Send(&qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "BTC/EUR",
			Bids:   []market.BookLevel{{Price: 10, Volume: 80}},
			Asks:   []market.BookLevel{{Price: 10.1, Volume: 20}},
		},
	})

	convey.Convey("Given a subscribed Hawkes symbol with book data", t, func() {
		convey.Convey("It should update imbalance on tick", func() {
			deadline := time.Now().Add(time.Second)
			var state *symbolState

			for time.Now().Before(deadline) {
				state = loadHawkesSymbol(signal, "BTC/EUR")

				if state != nil && state.readSnapshot().imbalance > 0 {
					break
				}

				time.Sleep(time.Millisecond)
			}

			convey.So(state, convey.ShouldNotBeNil)
			convey.So(state.readSnapshot().imbalance, convey.ShouldBeGreaterThan, 0)
		})
	})
}
