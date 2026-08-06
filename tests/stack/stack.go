package stack

import (
	"testing"

	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
WithStack runs one test against the real system wired to a simulated venue.

The market provides data and nothing else, so the stack is assembled here by the
same Boot the binary uses rather than by anything the fixtures own. That keeps
the venue free of every package it feeds, which is what lets those packages test
against a market at all, and it means a test exercises the system that ships
rather than a smaller one standing in for it.

It lives beside the fixtures rather than inside them because Boot reaches every
stage, and a venue that imported it could not be imported back by them.
*/
func WithStack(
	t *testing.T,
	symbols []*testtypes.Symbol,
	f func(*tests.Market, *cmd.System),
) func() {
	return tests.WithMarket(t, symbols, func(market *tests.Market) {
		public, private := market.Feeds()
		system := cmd.Boot(t.Context(), types.NewThesis(nil), public, private, nil)

		if system == nil {
			t.Fatal("stack: boot produced no system")
		}

		defer system.Close()

		f(market, system)
	})
}

/*
WithOrders is WithStack against the simulated private REST transport, for tests
that submit orders rather than only observing.
*/
func WithOrders(
	t *testing.T,
	symbols []*testtypes.Symbol,
	f func(*tests.Market, *cmd.System),
) func() {
	return tests.WithFixtureOrders(t, symbols, func(market *tests.Market) {
		public, private := market.Feeds()
		system := cmd.Boot(t.Context(), types.NewThesis(nil), public, private, nil)

		if system == nil {
			t.Fatal("stack: boot produced no system")
		}

		defer system.Close()

		f(market, system)
	})
}

/*
EntryRisk derives the stop geometry an entry for this symbol would be sized
under, from the live simulated book.

The desk refuses an entry without one: a quantity was solved against a
particular risk distance, and a lot fitted with some other distance after the
fact carries a loss nobody budgeted.
*/
func EntryRisk(system *cmd.System, symbol string) types.RiskPlan {
	pair, err := system.Desk.Instrument().Pair(symbol)

	if err != nil {
		return types.RiskPlan{}
	}

	return system.Desk.Price().RiskPlan(pair)
}

var _ = kraken.TickerData{}
