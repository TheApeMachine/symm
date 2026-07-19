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
			So(frame, ShouldContainSubstring, `"holdings"`)
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

func TestSyncWalletUsesAvailable(t *testing.T) {
	Convey("Given locked base inventory with zero Available", t, func() {
		holdings := &sync.Map{}
		holdings.Store("BTC/USD", &types.Holding{
			Symbol: "BTC/USD",
			Asset:  "BTC",
			Qty:    decimal.NewFromFloat64(0.0004675),
			Status: types.OPEN,
		})
		cash := decimal.NewFromFloat64(100)
		locked := decimal.NewFromFloat64(0.0004675)
		zero := decimal.NewFromFloat64(0)
		balance := &Balance{
			status:   types.READY,
			quote:    "USD",
			holdings: holdings,
			model: &kraken.Balance{Data: []kraken.BalanceData{
				{Asset: "USD", Balance: cash, Available: cash},
				{Asset: "BTC", Balance: locked, Available: zero},
			}},
		}

		balance.syncWallet()

		Convey("Then inventory stays open on total Balance with zero SellableQty", func() {
			value, ok := balance.holdings.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			holding := value.(*types.Holding)
			So(holding.Status, ShouldEqual, types.OPEN)
			So(holding.Qty.Float64(), ShouldAlmostEqual, 0.0004675, 1e-12)
			So(holding.SellableQty.Sign(), ShouldEqual, 0)
		})
	})
}

func TestSyncWalletClosesAbsent(t *testing.T) {
	Convey("Given a phantom lot absent from the wallet snapshot", t, func() {
		holdings := &sync.Map{}
		holdings.Store("ONDO/USD", &types.Holding{
			Symbol: "ONDO/USD",
			Asset:  "ONDO",
			Qty:    decimal.NewFromFloat64(113.9),
			Status: types.OPEN,
		})
		cash := decimal.NewFromFloat64(199.41)
		balance := &Balance{
			status:   types.READY,
			quote:    "USD",
			holdings: holdings,
			model: &kraken.Balance{Data: []kraken.BalanceData{{
				Asset: "USD", Balance: cash, Available: cash,
			}}},
		}

		balance.syncWallet()

		Convey("Then the phantom lot is closed and retained", func() {
			value, ok := balance.holdings.Load("ONDO/USD")
			So(ok, ShouldBeTrue)
			So(value.(*types.Holding).Status, ShouldEqual, types.CLOSED)
		})
	})
}

/*
TestBalanceAckPaperSnapshotDropsOmittedAssets covers the paper wallet shape:
`kraken paper balance` returns only non-zero assets. After a sell the next frame
must replace the model (snapshot) so stale positive Available rows cannot keep
phantom OPEN lots alive and inflate equity.
*/
func TestBalanceAckPaperSnapshotDropsOmittedAssets(t *testing.T) {
	Convey("Given a wallet that still carries a sold base asset", t, func() {
		ethQty := decimal.NewFromFloat64(0.01669712)
		usd := decimal.NewFromFloat64(165.0)
		holdings := &sync.Map{}
		holdings.Store("ETH/USD", &types.Holding{
			Symbol:     "ETH/USD",
			Asset:      "ETH",
			Qty:        ethQty.Copy(),
			EntryPrice: decimal.NewFromFloat64(1859.86),
			Status:     types.OPEN,
		})
		balance := &Balance{
			status:   types.READY,
			quote:    "USD",
			holdings: holdings,
			model: &kraken.Balance{
				Type:     "snapshot",
				Sequence: 1,
				Data: []kraken.BalanceData{
					{Asset: "USD", Balance: usd, Available: usd},
					{Asset: "ETH", Balance: ethQty, Available: ethQty.Copy()},
				},
			},
		}

		Convey("When a paper-style cash-only snapshot arrives", func() {
			cash := decimal.NewFromFloat64(196.76)
			payload, err := (&kraken.Balance{
				Channel:  "balances",
				Type:     "snapshot",
				Sequence: 2,
				Data: []kraken.BalanceData{{
					Asset: "USD", Balance: cash, Available: cash,
				}},
			}).MarshalJSON()
			So(err, ShouldBeNil)

			balance.BalanceAck(payload)

			Convey("Then the omitted base asset lot is closed", func() {
				value, ok := balance.holdings.Load("ETH/USD")
				So(ok, ShouldBeTrue)
				So(value.(*types.Holding).Status, ShouldEqual, types.CLOSED)
				So(value.(*types.Holding).Qty.Sign(), ShouldEqual, 0)
				So(len(balance.model.Data), ShouldEqual, 1)
				So(balance.model.Data[0].Asset, ShouldEqual, "USD")
			})
		})

		Convey("When the same cash-only frame arrives as an update instead", func() {
			cash := decimal.NewFromFloat64(196.76)
			payload, err := (&kraken.Balance{
				Channel:  "balances",
				Type:     "update",
				Sequence: 2,
				Data: []kraken.BalanceData{{
					Asset: "USD", Balance: cash, Available: cash,
				}},
			}).MarshalJSON()
			So(err, ShouldBeNil)

			balance.BalanceAck(payload)

			Convey("Then the stale ETH row keeps the phantom lot open", func() {
				value, ok := balance.holdings.Load("ETH/USD")
				So(ok, ShouldBeTrue)
				So(value.(*types.Holding).Status, ShouldEqual, types.OPEN)
				So(value.(*types.Holding).Qty.Sign(), ShouldEqual, 1)
			})
		})
	})
}

func TestRememberRequiresWalletQty(t *testing.T) {
	Convey("Given a live wallet without the recovered asset", t, func() {
		cash := decimal.NewFromFloat64(100)
		balance := &Balance{
			status:   types.READY,
			quote:    "USD",
			holdings: &sync.Map{},
			model: &kraken.Balance{Data: []kraken.BalanceData{{
				Asset: "USD", Balance: cash, Available: cash,
			}}},
		}

		balance.Remember(&types.Holding{
			Symbol: "ONDO/USD",
			Asset:  "ONDO",
			Qty:    decimal.NewFromFloat64(10),
			Status: types.OPEN,
		})

		Convey("Then Remember refuses to invent inventory", func() {
			_, ok := balance.holdings.Load("ONDO/USD")
			So(ok, ShouldBeFalse)
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
