package integration

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
deskHarness centralizes production boot wiring for Desk integration tests.
It owns the simulated market and wired stack so scenarios share one setup path.
*/
type deskHarness struct {
	Market *tests.Market
	Wired  *stack.Stack
}

/*
newDeskHarness boots the production graph on a simulated market.
*/
func newDeskHarness(testingContext testing.TB, symbolCount int) *deskHarness {
	testingContext.Helper()

	market := tests.NewMarket(testingContext.Context(), symbolCount)
	wired, err := stack.NewBooter(testingContext.Context()).Test(market)

	if err != nil {
		testingContext.Fatal(err)
	}

	return &deskHarness{
		Market: market,
		Wired:  wired,
	}
}

/*
reset closes wired resources and the simulated market for Convey teardown.
*/
func (harness *deskHarness) reset() {
	So(harness.Wired.Close(), ShouldBeNil)
	harness.Market.Close()
}

/*
Warmup runs the idle tape leg so fixtures and actors reach READY state.
*/
func (harness *deskHarness) Warmup() error {
	return harness.Market.Warmup(tests.Idle)
}

/*
sellAllOpen closes every open lot through Desk.Sell, optionally keeping symbols.
*/
func sellAllOpen(desk *broker.Desk, balance *broker.Balance, keepSymbol ...string) {
	keep := make(map[string]struct{}, len(keepSymbol))

	for _, symbol := range keepSymbol {
		keep[symbol] = struct{}{}
	}

	for _, open := range balance.Holdings() {
		if open.Status != types.OPEN {
			continue
		}

		if _, retained := keep[open.Symbol]; retained {
			continue
		}

		So(desk.Sell(open.Symbol), ShouldBeNil)
	}
}
