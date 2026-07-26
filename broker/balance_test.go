package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/types"
)

func TestBalanceAck(t *testing.T) {
	Convey("Given a balance that accepted snapshot sequence 1", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 2), config.Fixture().Market)
		balance.quote = "USD"
		balance.BalanceAck([]byte(`{
			"channel":"balances","type":"snapshot","sequence":1,
			"data":[{"asset":"USD","balance":1000}]
		}`))

		Convey("It accepts the exact next update sequence", func() {
			balance.BalanceAck([]byte(`{
				"channel":"balances","type":"update","sequence":2,
				"data":[{"asset":"USD","balance":900}]
			}`))
			row, err := balance.Get("USD")
			So(err, ShouldBeNil)
			So(row.Balance.Float64(), ShouldEqual, 900.0)
		})

		Convey("It rejects a gapped update without applying it", func() {
			balance.BalanceAck([]byte(`{
				"channel":"balances","type":"update","sequence":4,
				"data":[{"asset":"USD","balance":1}]
			}`))
			row, err := balance.Get("USD")
			So(err, ShouldBeNil)
			So(row.Balance.Float64(), ShouldEqual, 1000.0)
		})
	})
}

func TestBalanceGet(t *testing.T) {
	Convey("Given a wallet snapshot with quote cash", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.quote = "USD"
		balance.BalanceAck([]byte(`{
			"channel":"balances","type":"snapshot","sequence":1,
			"data":[{"asset":"USD","balance":500}]
		}`))

		Convey("Get returns an independent copy of the row", func() {
			row, err := balance.Get("USD")
			So(err, ShouldBeNil)
			So(row.Balance.Float64(), ShouldEqual, 500.0)
			row.Balance = row.Balance.Add(row.Balance)
			again, err := balance.Get("USD")
			So(err, ShouldBeNil)
			So(again.Balance.Float64(), ShouldEqual, 500.0)
		})
	})
}

func TestBalanceRecoverySeed(t *testing.T) {
	Convey("Given recovered holdings from durable restart state", t, func() {
		qty := decimal.NewFromFloat64(2)
		balance := NewBalance(nil, []types.Holding{{
			Symbol: "BTC/USD",
			Asset:  "BTC",
			Qty:    qty,
			Status: types.OPEN,
		}}, make(chan []byte, 1), config.Fixture().Market)

		Convey("Then the inventory should expose the holding before any wallet snapshot", func() {
			holding, err := balance.Holding("BTC/USD")

			So(err, ShouldBeNil)
			So(holding.Asset, ShouldEqual, "BTC")
			So(holding.Qty.String(), ShouldEqual, qty.String())
		})
	})
}

func TestCashFreeCash(t *testing.T) {
	Convey("Given quote cash and an open cash reservation", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.quote = "USD"
		balance.BalanceAck([]byte(`{
			"channel":"balances","type":"snapshot","sequence":1,
			"data":[{"asset":"USD","balance":1000}]
		}`))
		So(balance.Reserve(
			"buy", "BTC/USD", decimal.NewFromInt64(50), true,
		), ShouldBeNil)

		Convey("FreeCash subtracts the reserved claim", func() {
			free, err := balance.FreeCash()
			So(err, ShouldBeNil)
			So(free.Float64(), ShouldEqual, 950.0)
		})

		Convey("Available is false when the amount exceeds free cash", func() {
			ok, err := balance.Available(decimal.NewFromInt64(960))
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)
		})
	})
}

func TestCashAssetAvailable(t *testing.T) {
	Convey("Given base inventory and a sell reservation", t, func() {
		balance := NewBalance(nil, nil, make(chan []byte, 1), config.Fixture().Market)
		balance.quote = "USD"
		balance.BalanceAck([]byte(`{
			"channel":"balances","type":"snapshot","sequence":1,
			"data":[
				{"asset":"USD","balance":1000},
				{"asset":"BTC","balance":2}
			]
		}`))
		So(balance.ReserveAsset(
			"sell", "BTC", decimal.NewFromInt64(1),
		), ShouldBeNil)

		Convey("AssetAvailable subtracts the reserved qty", func() {
			available, err := balance.AssetAvailable("BTC")
			So(err, ShouldBeNil)
			So(available.Float64(), ShouldEqual, 1.0)
		})
	})
}
