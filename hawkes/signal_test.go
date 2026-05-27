package hawkes

import (
	"context"
	"testing"

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
		imbalance: 0.5,
	})

	pool.CreateBroadcastGroup("book", 0).Send(&qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "BTC/EUR",
			Bids:   []market.BookLevel{{Price: 10, Volume: 80}},
			Asks:   []market.BookLevel{{Price: 10.1, Volume: 20}},
		},
	})

	convey.Convey("Given a subscribed Hawkes symbol with book data", t, func() {
		convey.Convey("It should update imbalance on tick", func() {
			convey.So(signal.Tick(), convey.ShouldBeNil)

			state := loadHawkesSymbol(signal, "BTC/EUR")

			convey.So(state, convey.ShouldNotBeNil)
			convey.So(state.imbalance, convey.ShouldBeGreaterThan, 0)
		})
	})
}
