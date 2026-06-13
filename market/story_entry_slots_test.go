package market

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/trader"
)

func TestStoryEntrySlotOccupancyCountsPendingBuys(t *testing.T) {
	Convey("Given six in-flight base entries and no confirmed holdings", t, func() {
		story := newAccountTestStory(t)
		tradingConfig := config.TradingConfig{
			MaxConcurrentPositions: 4,
			OpportunitySlotCount:   2,
		}

		for index := range 6 {
			story.markPendingIntent(&logic.Action{
				Type:            logic.ActionMarket,
				Side:            trading.Buy,
				Symbol:          fmt.Sprintf("SYM%d/USD", index),
				EntryConfidence: 0.8,
			})
		}

		occupancy := story.entrySlotOccupancy()

		Convey("It should treat pending buys as committed slots", func() {
			So(occupancy.BasePending, ShouldEqual, 6)
			So(occupancy.CommittedCount(), ShouldEqual, 6)
		})

		Convey("It should reject another entry", func() {
			allowed, _ := logic.EntrySlotAdmission(
				occupancy,
				tradingConfig,
				false,
			)

			So(allowed, ShouldBeFalse)
		})
	})
}

func TestPrepareActionRejectsSeventhPendingEntry(t *testing.T) {
	Convey("Given six committed entry slots", t, func() {
		holdings := logic.NewHoldings()
		occupancy := logic.EntrySlotOccupancy{
			BasePending:        4,
			OpportunityPending: 2,
		}

		tradingConfig := config.TradingConfig{
			Model:                  "paper",
			MaxConcurrentPositions: 4,
			OpportunitySlotCount:   2,
		}

		capitalProvider, providerErr := trader.NewStaticCapitalProvider(200)

		So(providerErr, ShouldBeNil)

		prepared, err := prepareAction(
			t.Context(),
			holdings,
			occupancy,
			&logic.Action{Type: logic.ActionMarket, Side: trading.Buy},
			[]logic.Measurement{
				logic.NewMeasurement(
					logic.SourceHawkes,
					"NEW/USD",
					1,
					1,
					1,
					1,
					1,
					logic.CategoryFrenzy,
					logic.RegimeTypeNone,
					logic.PositionTypeNone,
					0.9,
					1,
				),
			},
			tradingConfig,
			logic.NewThresholdContext(0.55, 0, 0),
			capitalProvider,
		)

		Convey("It should withhold the entry", func() {
			So(err, ShouldBeNil)
			So(prepared, ShouldBeNil)
		})
	})
}
