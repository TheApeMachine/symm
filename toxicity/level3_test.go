package toxicity

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
)

func TestToxicityObserveLevel3(t *testing.T) {
	Convey("Given L3 add/delete events at the touch", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		tox := NewToxicity(ctx, pool)
		defer tox.Close()

		now := time.Now()
		symbol := "BTC/EUR"
		price := 100.0

		tox.tracker.ObserveMid(symbol, market.Pair{}, price)
		state := tox.tracker.stateLocked(symbol, market.Pair{})
		state.bidTotal = 100

		update := &market.Level3Update{
			Symbol: symbol,
			Bids: []market.Level3OrderEvent{
				{Event: "add", OrderID: "l3-1", LimitPrice: price, OrderQty: 15, Timestamp: now.Format(time.RFC3339Nano)},
				{Event: "delete", OrderID: "l3-1", LimitPrice: price, OrderQty: 15, Timestamp: now.Format(time.RFC3339Nano)},
			},
		}

		tox.observeLevel3(update)

		Convey("It should flag toxic bluff from per-order churn", func() {
			So(tox.tracker.IsToxic(symbol, price, now), ShouldBeTrue)
		})
	})
}
