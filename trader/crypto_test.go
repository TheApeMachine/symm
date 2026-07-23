package trader_test

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
TestCryptoApplyEnterExit proves Crypto.Apply submits a sized enter through Desk,
fills on the paper venue, then exits the full open lot — no partial reduce.
*/
func TestCryptoApplyEnterExit(t *testing.T) {
	Convey("Given a warmed production graph on one symbol", t, func() {
		market := tests.NewMarket(t.Context(), 1)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			So(wired.Close(), ShouldBeNil)
			market.Close()
		})
		So(market.Warmup(tests.Idle), ShouldBeNil)
		symbol := market.Symbols[0]
		quantity := decimal.NewFromInt64(2)

		Convey("When Apply receives a sized enter decision", func() {
			thesis := wired.Thesis
			holding := types.NewHolding(t.Context(), symbol, quantity)
			holding.IsOpportunity = true
			thesis.Holdings.Store(symbol, holding)
			thesis.Decisions = []types.Decision{{
				Action:           types.ActionEnter,
				Symbol:           symbol,
				ProposedQuantity: quantity,
				At:               market.Now(),
			}}

			wired.Crypto.Apply(thesis)
			So(market.Paper.Drain(), ShouldBeNil)

			Convey("Then the lot is open at the exact proposed quantity", func() {
				phase, found := thesis.Lifecycle.Load(symbol)
				So(found, ShouldBeTrue)
				So(phase, ShouldEqual, types.LifecycleEntrySubmitted)
				open, opened := waitHolding(wired, symbol, types.OPEN)
				So(opened, ShouldBeTrue)
				So(wired.Desk.OpenPositions(), ShouldEqual, 1)
				So(open.Qty.Cmp(quantity), ShouldEqual, 0)
				So(open.EntryPrice, ShouldNotBeNil)
				So(open.EntryPrice.Sign(), ShouldEqual, 1)
				So(open.EntryFee, ShouldNotBeNil)
				So(open.EntryFee.Sign(), ShouldEqual, 1)

				Convey("When Apply receives an exit for that symbol", func() {
					thesis.Decisions = []types.Decision{{
						Action: types.ActionExit,
						Symbol: symbol,
						At:     market.Now(),
					}}
					wired.Crypto.Apply(thesis)
					So(market.Paper.Drain(), ShouldBeNil)

					Convey("Then the lot is fully closed", func() {
						phase, found = thesis.Lifecycle.Load(symbol)
						So(found, ShouldBeTrue)
						So(phase, ShouldEqual, types.LifecycleExitSubmitted)
						So(waitGone(wired, symbol), ShouldBeTrue)
						So(wired.Desk.OpenPositions(), ShouldEqual, 0)
					})
				})
			})
		})
	})
}

/*
waitHolding polls Balance until the symbol reaches status or the deadline elapses.
Drain only Enqueues Actor work; Desk ExecutionAck is what opens the lot.
*/
func waitHolding(
	wired *stack.Stack, symbol string, status types.Status,
) (types.Holding, bool) {
	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		holding, err := wired.Balance.Holding(symbol)

		if err == nil && holding.Status == status {
			return holding, true
		}

		time.Sleep(5 * time.Millisecond)
	}

	return types.Holding{}, false
}

/*
waitGone polls until Balance no longer reports an open lot for symbol.
*/
func waitGone(wired *stack.Stack, symbol string) bool {
	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		if _, err := wired.Balance.Holding(symbol); err != nil {
			return true
		}

		time.Sleep(5 * time.Millisecond)
	}

	return false
}

/*
BenchmarkCryptoApplyEnter measures one sized enter through Desk on paper.
*/
func BenchmarkCryptoApplyEnter(b *testing.B) {
	market := tests.NewMarket(b.Context(), 1)
	wired, err := stack.NewBooter(b.Context()).Test(market)

	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		_ = wired.Close()
		market.Close()
	}()

	if err := market.Warmup(tests.Idle); err != nil {
		b.Fatal(err)
	}

	symbol := market.Symbols[0]
	quantity := decimal.NewFromInt64(1)
	b.ReportAllocs()

	for b.Loop() {
		thesis := wired.Thesis
		holding := types.NewHolding(b.Context(), symbol, quantity)
		holding.IsOpportunity = true
		thesis.Holdings.Store(symbol, holding)
		thesis.Decisions = []types.Decision{{
			Action:           types.ActionEnter,
			Symbol:           symbol,
			ProposedQuantity: quantity,
			At:               market.Now(),
		}}
		wired.Crypto.Apply(thesis)

		if err := market.Paper.Drain(); err != nil {
			b.Fatal(err)
		}

		if err := wired.Desk.Sell(symbol); err != nil {
			b.Fatal(err)
		}

		if err := market.Paper.Drain(); err != nil {
			b.Fatal(err)
		}
	}
}
