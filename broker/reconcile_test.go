package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestPositionRestorePending verifies boot reconcile re-arms an outstanding order
intent from a durable wire without touching the venue, so a recovered lot reads
as pending and Trader will not re-submit.
*/
func TestPositionRestorePending(t *testing.T) {
	Convey("Given a recovered lot with a durable exit intent", t, func() {
		balance := &Balance{quote: "USD", holdings: &sync.Map{}}
		position := &Position{
			status:  types.OPEN,
			balance: balance,
			pair:    &kraken.InstrumentPair{Symbol: "BTC/USD", Base: "BTC"},
		}
		position.intent.Store(intentFlat)

		position.RestorePending(types.PendingOrderWire{
			Symbol:        "BTC/USD",
			Side:          "sell",
			OrderID:       "order-restored",
			Intent:        IntentExitPending,
			ReservationID: "resv-1",
		})

		Convey("Then the lot reports pending without a new venue order", func() {
			So(position.Pending(), ShouldBeTrue)
			So(position.Status(), ShouldEqual, types.PENDING)
			So(position.orderID, ShouldEqual, "order-restored")
			So(position.claim.ID(), ShouldEqual, "resv-1")
		})
	})

	Convey("Given an empty pending wire", t, func() {
		position := &Position{status: types.OPEN}
		position.intent.Store(intentFlat)

		position.RestorePending(types.PendingOrderWire{})

		Convey("Then the lot stays flat", func() {
			So(position.Pending(), ShouldBeFalse)
		})
	})
}

/*
TestDeskReconcilePending verifies the desk restores only those pending intents
whose order id still rests on the exchange, leaving stale intents for executions
and wallet sync to resolve.
*/
func TestDeskReconcilePending(t *testing.T) {
	Convey("Given a recovered lot wrapped in a desk Position", t, func() {
		balance := &Balance{quote: "USD", holdings: &sync.Map{}}
		positions := &sync.Map{}
		position := &Position{
			status:  types.OPEN,
			balance: balance,
			pair:    &kraken.InstrumentPair{Symbol: "BTC/USD", Base: "BTC"},
		}
		position.intent.Store(intentFlat)
		positions.Store("BTC/USD", position)

		desk := &Desk{
			positions: positions,
			marks:     Marks{positions: positions},
			balance:   balance,
		}

		pending := map[string]types.PendingOrderWire{
			"BTC/USD": {
				Symbol:  "BTC/USD",
				Side:    "sell",
				OrderID: "open-1",
				Intent:  IntentExitPending,
			},
		}

		Convey("When the wire matches a resting exchange order", func() {
			desk.ReconcilePending(pending, map[string]spot.Order{"open-1": {}})

			Convey("Then the position is re-armed as pending without re-entry", func() {
				So(position.Pending(), ShouldBeTrue)
				So(position.orderID, ShouldEqual, "open-1")
			})
		})

		Convey("When no resting order matches the wire", func() {
			desk.ReconcilePending(pending, map[string]spot.Order{})

			Convey("Then the stale intent is left for executions sync", func() {
				So(position.Pending(), ShouldBeFalse)
			})
		})
	})
}
