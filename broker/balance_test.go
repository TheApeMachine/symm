package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

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
				Amount:    total,
				Balance:   total,
				Available: available,
			}}},
		}

		Convey("It should authorize from available cash, not total balance", func() {
			So(balance.Available(requested), ShouldBeFalse)
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
		holdings := &sync.Map{}
		holdings.Store("BTC/USD", &types.Holding{
			Symbol: "BTC/USD",
			Qty:    decimal.NewFromFloat64(0.01),
			Status: types.OPEN,
		})
		balance := &Balance{
			status:   types.READY,
			quote:    "USD",
			ui:       messages,
			holdings: holdings,
			model: &kraken.Balance{Data: []kraken.BalanceData{{
				Asset:     "USD",
				Balance:   total,
				Available: available,
				Reserved:  reserved,
			}}},
		}

		balance.Publish()

		Convey("It should emit balances and open holdings together", func() {
			So(len(messages), ShouldEqual, 1)
			frame := string(<-messages)
			So(frame, ShouldContainSubstring, `"balances"`)
			So(frame, ShouldContainSubstring, `"positions"`)
			So(frame, ShouldContainSubstring, `"BTC/USD"`)
			So(frame, ShouldContainSubstring, `"balance":100`)
			So(frame, ShouldContainSubstring, `"available":90`)
			So(frame, ShouldContainSubstring, `"reserved":10`)
			So(frame, ShouldNotContainSubstring, `"balances":100`)
			So(balance.Snapshot(), ShouldHaveLength, 1)
		})
	})
}

/*
TestBalanceFrameRequiresModel keeps reconnect snapshots from wiping the UI with
an empty balances array while Kraken resync is still in flight.
*/
func TestBalanceFrameRequiresModel(t *testing.T) {
	Convey("Given a ready balance with a cleared model", t, func() {
		balance := &Balance{
			status: types.READY,
			quote:  "USD",
			ui:     make(chan []byte, 1),
		}

		Convey("Then Publish refuses to publish", func() {
			balance.Publish()
		})
	})
}

func BenchmarkBalanceSnapshot(b *testing.B) {
	total, _ := decimal.NewFromString("100")
	balance := &Balance{
		status: types.READY,
		quote:  "USD",
		model: &kraken.Balance{Data: []kraken.BalanceData{{
			Asset: "USD", Balance: total, Available: total,
		}}},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = balance.Snapshot()
	}
}

/*
BenchmarkBalanceAck measures balance snapshot ingestion and publication.
*/
func BenchmarkBalanceAck(b *testing.B) {
	model := &kraken.Balance{Channel: "balances", Data: []kraken.BalanceData{
		{
			Asset:     "USD",
			Balance:   decimal.NewFromInt64(50),
			Available: decimal.NewFromInt64(50),
			Reserved:  decimal.NewFromInt64(0),
		},
		{
			Asset:     "ZEC",
			Balance:   decimal.NewFromFloat64(0.2),
			Available: decimal.NewFromFloat64(0.2),
			Reserved:  decimal.NewFromInt64(0),
		},
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
