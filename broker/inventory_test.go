package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/types"
	. "github.com/smartystreets/goconvey/convey"
)

func TestInventorySync(t *testing.T) {
	Convey("Given a wallet snapshot with non-quote inventory", t, func() {
		ui := make(chan []byte, 1)
		balance := NewBalance(nil, nil, ui, config.Fixture().Market)
		balance.quote = "USD"
		frame := []byte(`{
			"channel":"balances","type":"snapshot","sequence":1,
			"data":[
				{"asset":"USD","balance":900},
				{"asset":"ETH","balance":2}
			]
		}`)

		Convey("When BalanceAck ingests the snapshot", func() {
			balance.BalanceAck(frame)

			Convey("It materializes an open holding for the existing lot", func() {
				holding, err := balance.Holding("ETH/USD")
				So(err, ShouldBeNil)
				So(holding.Status, ShouldEqual, types.OPEN)
				So(holding.Qty.Float64(), ShouldEqual, 2.0)
				So(len(ui), ShouldEqual, 1)
				payload := <-ui
				So(string(payload), ShouldContainSubstring, `"ETH/USD"`)
			})
		})
	})
}

func TestInventoryUpdate(t *testing.T) {
	Convey("Given an open lot in inventory", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.StoreHolding(&types.Holding{
			Symbol: "BTC/USD",
			Asset:  "BTC",
			Qty:    decimal.NewFromFloat64(0.5),
			Status: types.OPEN,
		})

		Convey("Update mutates the live lot under the wallet lock", func() {
			err := balance.Update("BTC/USD", func(holding *types.Holding) error {
				holding.Qty = decimal.NewFromFloat64(1.25)

				return nil
			})
			So(err, ShouldBeNil)
			holding, err := balance.Holding("BTC/USD")
			So(err, ShouldBeNil)
			So(holding.Qty.Float64(), ShouldEqual, 1.25)
		})
	})
}

func TestInventoryHoldings(t *testing.T) {
	Convey("Given open and closed lots", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.StoreHolding(&types.Holding{
			Symbol: "AAA/USD",
			Qty:    decimal.NewFromFloat64(1),
			Status: types.OPEN,
		})
		balance.StoreHolding(&types.Holding{
			Symbol: "BBB/USD",
			Qty:    decimal.NewFromFloat64(0),
			Status: types.CLOSED,
		})

		Convey("Holdings yields only open lots", func() {
			symbols := make([]string, 0)

			for holding := range balance.Holdings() {
				symbols = append(symbols, holding.Symbol)
			}

			So(symbols, ShouldResemble, []string{"AAA/USD"})
		})
	})
}
