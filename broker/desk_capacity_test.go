package broker

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
)

func TestDeskValidateEntryCapacity(t *testing.T) {
	Convey("Given a desk at six committed slots", t, func() {
		desk := &Desk{
			tradingConfig: config.TradingConfig{
				MaxConcurrentPositions: 4,
				OpportunitySlotCount:   2,
			},
			positions: NewPositionMonitor(),
			actions:   &sync.Map{},
		}

		inventory := make(map[string]float64, 4)

		for index := range 4 {
			inventory[fmt.Sprintf("BASE%d", index)] = 1
		}

		desk.positions.ApplyBalance(user.Balances{
			Currency:  "USD",
			Balance:   100,
			Inventory: inventory,
		})

		for index := range 2 {
			desk.actions.Store(
				fmt.Sprintf("pending-%d", index),
				&logic.Action{
					Type:   logic.ActionMarket,
					Side:   trading.Buy,
					Symbol: fmt.Sprintf("PEND%d/USD", index),
				},
			)
		}

		err := desk.validateEntryCapacity(&logic.Action{
			Type:   logic.ActionMarket,
			Side:   trading.Buy,
			Symbol: "NEW/USD",
		})

		Convey("It should reject another buy", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
