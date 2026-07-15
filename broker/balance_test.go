package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestBalanceAckRefreshesSymbolHolding verifies wallet frames refresh both the
asset inventory and the managed symbol Holding without losing execution state.
*/
func TestBalanceAckRefreshesSymbolHolding(t *testing.T) {
	Convey("Given a balance frame after an execution", t, func() {
		holdings := &sync.Map{}
		holdings.Store("ZEC/USD", types.Holding{
			Symbol: "ZEC/USD", Asset: "ZEC",
			Qty: *decimal.NewFromInt64(0),
		})
		balance := &Balance{
			quote: "USD", holdings: holdings,
			ui: make(chan []byte, 1),
		}
		model := &kraken.Balance{Channel: "balances", Data: []kraken.BalanceData{
			{Asset: "USD", Balance: *decimal.NewFromInt64(50), Available: *decimal.NewFromInt64(50)},
			{Asset: "ZEC", Balance: *decimal.NewFromFloat64(0.2), Available: *decimal.NewFromFloat64(0.2)},
		}}
		buffer, err := model.MarshalJSON()

		So(err, ShouldBeNil)
		balance.BalanceAck(buffer)

		Convey("It should refresh both wallet inventory and the managed symbol", func() {
			holding, holdingErr := balance.Holding("ZEC/USD")
			wallet, walletErr := balance.Holding("ZEC")

			So(holdingErr, ShouldBeNil)
			So(walletErr, ShouldBeNil)
			So(holding.Qty.Float64(), ShouldEqual, 0.2)
			So(wallet.Qty.Float64(), ShouldEqual, 0.2)
			So(balance.Snapshot()[0]["balance"], ShouldEqual, 50.0)
		})
	})
}

func TestBalanceAvailable(t *testing.T) {
	Convey("Given reserved quote cash", t, func() {
		total, err := decimal.NewFromString("100")
		So(err, ShouldBeNil)
		available, err := decimal.NewFromString("10")
		So(err, ShouldBeNil)
		requested, err := decimal.NewFromString("50")
		So(err, ShouldBeNil)
		balance := &Balance{
			status: types.READY,
			quote:  "USD",
			model: &kraken.Balance{Data: []kraken.BalanceData{{
				Asset:     "USD",
				Amount:    *total,
				Balance:   *total,
				Available: *available,
			}}},
		}

		Convey("It should authorize from available cash, not total balance", func() {
			So(balance.Available(*requested), ShouldBeFalse)
		})
	})
}

func TestBalancePublish(t *testing.T) {
	Convey("Given an observed ready balance", t, func() {
		total, err := decimal.NewFromString("100")
		So(err, ShouldBeNil)
		available, err := decimal.NewFromString("90")
		So(err, ShouldBeNil)
		reserved, err := decimal.NewFromString("10")
		So(err, ShouldBeNil)
		messages := make(chan []byte, 1)
		balance := &Balance{
			status: types.READY,
			quote:  "USD",
			ui:     messages,
			model: &kraken.Balance{Data: []kraken.BalanceData{{
				Asset:     "USD",
				Balance:   *total,
				Available: *available,
				Reserved:  *reserved,
			}}},
		}

		balance.Publish()

		Convey("It should emit and notify the portfolio owner", func() {
			So(len(messages), ShouldEqual, 1)
			So(<-messages, ShouldNotBeEmpty)
			So(balance.Snapshot(), ShouldHaveLength, 1)
		})
	})
}

func BenchmarkBalanceSnapshot(b *testing.B) {
	total, _ := decimal.NewFromString("100")
	balance := &Balance{
		status: types.READY,
		quote:  "USD",
		model: &kraken.Balance{Data: []kraken.BalanceData{{
			Asset: "USD", Balance: *total, Available: *total,
		}}},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = balance.Snapshot()
	}
}

/*
BenchmarkBalanceAck measures wallet-to-managed-Holding synchronization.
*/
func BenchmarkBalanceAck(b *testing.B) {
	model := &kraken.Balance{Channel: "balances", Data: []kraken.BalanceData{
		{Asset: "USD", Balance: *decimal.NewFromInt64(50), Available: *decimal.NewFromInt64(50)},
		{Asset: "ZEC", Balance: *decimal.NewFromFloat64(0.2), Available: *decimal.NewFromFloat64(0.2)},
	}}
	buffer, err := model.MarshalJSON()

	if err != nil {
		b.Fatal(err)
	}

	balance := &Balance{
		quote: "USD", holdings: &sync.Map{},
		ui: make(chan []byte, 1),
	}
	b.ReportAllocs()

	for b.Loop() {
		select {
		case <-balance.ui:
		default:
		}

		balance.BalanceAck(buffer)
	}
}
