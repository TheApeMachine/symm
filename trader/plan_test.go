package trader

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

func TestSyncInventoryJournalsEntered(t *testing.T) {
	previousQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() { viper.Set("market.quote_currency", previousQuote) })

	Convey("Given a wallet-backed open lot", t, func() {
		balance := broker.NewBalance(nil, nil, nil)
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"100","available":"100","reserved":"0"` +
				`},{` +
				`"asset":"BTC","balance":"0.01","available":"0.01","reserved":"0"` +
				`}]}`,
		))

		crypto := &Crypto{balance: balance}
		thesis := types.NewThesis(nil, nil)

		crypto.syncInventory(thesis)

		Convey("Then lifecycle is managing after the entered transition", func() {
			phase, ok := thesis.Lifecycle.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, types.LifecycleManaging)

			crypto.syncInventory(thesis)
			phase, ok = thesis.Lifecycle.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, types.LifecycleManaging)
		})

		Convey("And Thesis.Holdings is not a wallet mirror", func() {
			_, ok := thesis.Holdings.Load("BTC/USD")
			So(ok, ShouldBeFalse)
		})
	})
}

func TestSyncInventoryWalletAuthority(t *testing.T) {
	previousQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() { viper.Set("market.quote_currency", previousQuote) })

	Convey("Given stale recovery for a flattened paper wallet", t, func() {
		balance := broker.NewBalance(nil, nil, nil)
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"199.41","available":"199.41","reserved":"0"` +
				`}]}`,
		))

		crypto := &Crypto{
			balance: balance,
			snapshot: &types.Recovery{
				Holdings: map[string]types.Holding{
					"ONDO/USD": {
						Symbol:     "ONDO/USD",
						Asset:      "ONDO",
						Qty:        decimal.NewFromFloat64(113.9),
						Status:     types.OPEN,
						EntryPrice: decimal.NewFromFloat64(0.4),
					},
				},
			},
			postMortem: &strategy.PostMortem{},
		}

		thesis := types.NewThesis(nil, nil)
		thesis.Holdings.Store("ONDO/USD", &types.Holding{
			Symbol: "ONDO/USD",
			Qty:    decimal.NewFromFloat64(113.9),
			Status: types.OPEN,
		})
		thesis.Lifecycle.Store("ONDO/USD", types.LifecycleManaging)

		crypto.syncInventory(thesis)

		Convey("Then recovery does not reintroduce qty and PostMortem evaluates", func() {
			So(crypto.snapshot, ShouldBeNil)
			phase, ok := thesis.Lifecycle.Load("ONDO/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, types.LifecycleEvaluated)
		})
	})
}

func TestSyncInventoryEnrichesWalletLot(t *testing.T) {
	previousQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() { viper.Set("market.quote_currency", previousQuote) })

	Convey("Given recovery metadata for a live wallet lot", t, func() {
		balance := broker.NewBalance(nil, nil, nil)
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"100","available":"100","reserved":"0"` +
				`},{` +
				`"asset":"ONDO","balance":"10","available":"10","reserved":"0"` +
				`}]}`,
		))

		crypto := &Crypto{
			balance: balance,
			snapshot: &types.Recovery{
				Holdings: map[string]types.Holding{
					"ONDO/USD": {
						Symbol:     "ONDO/USD",
						EntryPrice: decimal.NewFromFloat64(0.55),
						Stoploss:   &types.Stoploss{Skill: types.Skill{Weight: 0.7}},
					},
				},
			},
		}
		thesis := types.NewThesis(nil, nil)

		crypto.syncInventory(thesis)

		Convey("Then entry economics land on the Balance lot", func() {
			holding, err := balance.Holding("ONDO/USD")
			So(err, ShouldBeNil)
			So(holding.EntryPrice, ShouldNotBeNil)
			So(holding.EntryPrice.Float64(), ShouldEqual, 0.55)
			So(holding.Stoploss, ShouldNotBeNil)
			So(holding.Stoploss.Weight, ShouldEqual, 0.7)
			So(crypto.snapshot, ShouldBeNil)
		})
	})
}

func TestMarkOpenPreservesExitSubmitted(t *testing.T) {
	previousQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() { viper.Set("market.quote_currency", previousQuote) })

	Convey("Given an in-flight exit on a wallet-backed lot", t, func() {
		balance := broker.NewBalance(nil, nil, nil)
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"100","available":"100","reserved":"0"` +
				`},{` +
				`"asset":"BTC","balance":"0.01","available":"0.01","reserved":"0"` +
				`}]}`,
		))

		crypto := &Crypto{balance: balance}
		thesis := types.NewThesis(nil, nil)
		thesis.Lifecycle.Store("BTC/USD", types.LifecycleExitSubmitted)

		crypto.syncInventory(thesis)

		Convey("Then exit_submitted survives when Desk cannot confirm abandonment", func() {
			phase, ok := thesis.Lifecycle.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, types.LifecycleExitSubmitted)
		})
	})
}

/*
TestHealAbandonedExitsReopensStuckRail clears ExitSubmitted when the lot is
still open and Desk has no pending order.
*/
func TestHealAbandonedExitsReopensStuckRail(t *testing.T) {
	previousQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	t.Cleanup(func() { viper.Set("market.quote_currency", previousQuote) })

	Convey("Given ExitSubmitted with open inventory and no pending order", t, func() {
		balance := broker.NewBalance(nil, nil, nil)
		balance.BalanceAck([]byte(
			`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
				`"asset":"USD","balance":"100","available":"100","reserved":"0"` +
				`},{` +
				`"asset":"ETH","balance":"0.01","available":"0.01","reserved":"0"` +
				`}]}`,
		))

		desk := broker.NewDesk(nil, nil, nil, balance)
		crypto := &Crypto{
			balance: balance,
			desk:    desk,
		}
		thesis := types.NewThesis(nil, nil)
		thesis.Lifecycle.Store("ETH/USD", types.LifecycleExitSubmitted)

		crypto.syncInventory(thesis)

		Convey("Then the phase returns to managing so Regulate can re-fire", func() {
			phase, ok := thesis.Lifecycle.Load("ETH/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, types.LifecycleManaging)
		})
	})
}

func BenchmarkSyncInventory(b *testing.B) {
	previousQuote := viper.GetString("market.quote_currency")
	viper.Set("market.quote_currency", "USD")
	b.Cleanup(func() { viper.Set("market.quote_currency", previousQuote) })

	balance := broker.NewBalance(nil, nil, nil)
	balance.BalanceAck([]byte(
		`{"channel":"balances","type":"snapshot","sequence":1,"data":[{` +
			`"asset":"USD","balance":"100","available":"100","reserved":"0"` +
			`},{` +
			`"asset":"BTC","balance":"0.01","available":"0.01","reserved":"0"` +
			`}]}`,
	))
	crypto := &Crypto{balance: balance, ctx: context.Background()}
	thesis := types.NewThesis(nil, nil)

	b.ReportAllocs()

	for b.Loop() {
		crypto.syncInventory(thesis)
	}
}
