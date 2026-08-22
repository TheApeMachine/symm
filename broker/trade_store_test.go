package broker

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestPositionStoreSaveTrade(t *testing.T) {
	Convey("Given an open position with entry economics", t, func() {
		store := newPositionStoreFixture(t)
		stoploss := newBrokerStoploss(t)
		entryTime := time.Now().UTC().Add(-5 * time.Minute)
		exitTime := time.Now().UTC()

		position := &Position{
			pair: kraken.InstrumentPair{
				Symbol: "ZEC/USD",
				Base:   "ZEC",
				Quote:  "USD",
			},
			Decision: types.Decision{
				ID:          "decision-zec-001",
				Symbol:      "ZEC/USD",
				ThesisScore: 0.88,
				GraphScore:  0.92,
				Cause:       "pump_momentum",
			},
			Holding: &types.Holding{
				Symbol:     "ZEC/USD",
				Asset:      "ZEC",
				Qty:        decimal.NewFromFloat64(5.5),
				EntryPrice: decimal.NewFromFloat64(42.50),
				EntryFee:   decimal.NewFromFloat64(0.12),
				EntryAt:    &entryTime,
				Mark:       decimal.NewFromFloat64(45.00),
				Stoploss:   stoploss,
			},
		}
		position.setStatus(types.OPEN)

		Convey("Saving the entry trade should record it in position_trades", func() {
			So(store.SaveTrade(position), ShouldBeNil)

			trades, err := store.RecentTrades(10)
			So(err, ShouldBeNil)
			So(len(trades), ShouldEqual, 1)
			So(trades[0].Decision.Id, ShouldEqual, "decision-zec-001")
			So(trades[0].Status, ShouldEqual, "open")

			Convey("Updating the trade upon exit should record exit economics and trigger reason", func() {
				position.Holding.ExitAt = &exitTime
				position.Holding.ExitPrice = decimal.NewFromFloat64(48.20)
				position.Holding.ExitFee = decimal.NewFromFloat64(0.14)
				position.Holding.PnL = decimal.NewFromFloat64(31.09)
				position.Holding.ReturnPct = 13.25
				position.Holding.Stoploss.TriggerReason = types.TriggerProtectedFloor
				position.Holding.Stoploss.TriggerMark = decimal.NewFromFloat64(48.15)
				position.setStatus(types.CLOSED)

				So(store.SaveTrade(position), ShouldBeNil)

				updatedTrades, err := store.RecentTrades(10)
				So(err, ShouldBeNil)
				So(len(updatedTrades), ShouldEqual, 1)
				So(updatedTrades[0].Status, ShouldEqual, "closed")
				So(updatedTrades[0].Holding.ReturnPct, ShouldAlmostEqual, 13.25, 0.01)
				So(updatedTrades[0].Holding.Stoploss.TriggerReason, ShouldEqual, types.TriggerProtectedFloor)
			})
		})
	})
}

func BenchmarkPositionStoreSaveTrade(b *testing.B) {
	store := newPositionStoreFixture(b)
	stoploss := newBrokerStoploss(b)
	entryTime := time.Now().UTC()

	position := &Position{
		pair: kraken.InstrumentPair{
			Symbol: "ZEC/USD",
			Base:   "ZEC",
			Quote:  "USD",
		},
		Decision: types.Decision{
			ID:          "bench-decision-001",
			Symbol:      "ZEC/USD",
			ThesisScore: 0.85,
		},
		Holding: &types.Holding{
			Symbol:     "ZEC/USD",
			Qty:        decimal.NewFromFloat64(1.0),
			EntryPrice: decimal.NewFromFloat64(50.0),
			EntryFee:   decimal.NewFromFloat64(0.1),
			EntryAt:    &entryTime,
			Stoploss:   stoploss,
		},
	}
	position.setStatus(types.OPEN)
	b.ResetTimer()

	for b.Loop() {
		if err := store.SaveTrade(position); err != nil {
			b.Fatalf("save trade failed: %v", err)
		}
	}
}
