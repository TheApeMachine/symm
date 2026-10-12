package broker

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests/tablestest"
)

type dummyTransport struct{}

func (d *dummyTransport) Read() ([]byte, error) {
	return nil, nil
}

func (d *dummyTransport) Write([]byte) error {
	return nil
}

func TestDesk(t *testing.T) {
	Convey("Given a Desk instance", t, func() {
		ctx := context.Background()
		desk := NewDesk(ctx, &dummyTransport{}, nil, nil)
		So(desk, ShouldNotBeNil)

		Convey("When Enter is called with a symbol and quantity", func() {
			qty := decimal.NewFromFloat64(0.5)
			err := desk.Enter("BTC/USD", qty)
			So(err, ShouldBeNil)

			val, ok := desk.positions.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			positions := val.([]*Position)
			So(len(positions), ShouldEqual, 1)

			Convey("When Apply is called with an execution", func() {
				desk.Apply(&kraken.Execution{
					Data: []kraken.ExecutionData{
						{
							Symbol:   "BTC/USD",
							ExecType: "trade",
							LastQty:  qty,
						},
					},
				})

				Convey("When Exit is called", func() {
					err := desk.Exit("BTC/USD")
					So(err, ShouldBeNil)
				})
			})
		})

		Convey("When initialized with pre-existing wallet holding", func() {
			balance := &Balance{
				Quote: "USD",
				wallet: &kraken.Balance{
					Data: []kraken.BalanceData{
						{
							Asset:   "USD",
							Balance: decimal.NewFromFloat64(160.14),
						},
						{
							Asset:   "ZAMA",
							Balance: decimal.NewFromFloat64(517.71),
						},
					},
				},
			}

			restoredDesk := NewDesk(ctx, &dummyTransport{}, nil, balance)
			So(restoredDesk.Holds("ZAMA/USD"), ShouldBeTrue)

			val, ok := restoredDesk.positions.Load("ZAMA/USD")
			So(ok, ShouldBeTrue)
			posList := val.([]*Position)
			So(len(posList), ShouldEqual, 1)
			So(posList[0].Qty().String(), ShouldEqual, "517.71")
			So(posList[0].Closed(), ShouldBeFalse)
		})

		Convey("When initialized with catalog containing closed positions", func() {
			cat := tablestest.New(t)
			So(cat.Ensure(ctx), ShouldBeNil)

			closedAt := time.Now().UTC().Truncate(time.Microsecond)
			openedAt := closedAt.Add(-10 * time.Minute)
			So(cat.RecordPosition(ctx, tables.PositionRecord{
				ID:            "historical-1",
				Symbol:        "ETH/USD",
				Status:        "closed",
				OpenedAt:      openedAt,
				ClosedAt:      &closedAt,
				Events:        `[{"kind":"entry_match"},{"kind":"exit"}]`,
				VenueCost:     2000.0,
				VenueProceeds: 2050.0,
				VenueFees:     2.0,
				VenueRealized: 48.0,
				HoldNs:        600 * 1e9,
				Outcome:       `{"venue_realized":48.0}`,
			}), ShouldBeNil)

			restoredDesk := NewDesk(ctx, &dummyTransport{}, nil, nil, cat)
			So(len(restoredDesk.Closed()), ShouldEqual, 1)
			So(restoredDesk.Closed()[0].ID(), ShouldEqual, "historical-1")
			So(restoredDesk.Closed()[0].Symbol(), ShouldEqual, "ETH/USD")
			So(restoredDesk.Closed()[0].Closed(), ShouldBeTrue)

			lc := restoredDesk.Closed()[0].Lifecycle()
			So(lc.Status, ShouldEqual, "closed")
			So(lc.Outcome, ShouldNotBeNil)
			So(lc.Outcome.VenueRealized, ShouldEqual, 48.0)
		})
	})
}
