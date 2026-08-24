package tests

import (
	"context"
	"testing"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	tes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
Stack is the running system a venue is driving.

Every package that owns a piece of the system tests against this venue, so the
venue cannot name the assembled system without closing a loop back through the
packages it feeds. It declares what it needs from whatever is consuming its
frames instead, and the caller hands the real system in. Nothing here reaches
past that: a venue that could see the whole system would start deciding things
the system under test is supposed to decide.
*/
type Stack interface {
	/*
		Holding reports how many lots the system currently carries for one
		symbol, which is what lets the venue run a position out to its close
		rather than for a number of ticks picked to make a test come out.
	*/
	Holding(symbol string) int
	Error() error
	Run() error
	Close() error
}

/*
Driven is a Stack a boot answers by value. Comparability is what separates an
assembled system from a boot that produced nothing, which would otherwise reach
the test body as a typed nil and fail somewhere further along than the cause.
*/
type Driven interface {
	comparable
	Stack
}

/*
Boot assembles a Stack around one canonical Thesis and a venue's connections.

This is the signature of cmd.Boot, which callers pass in directly rather than
adapt: a test then exercises the system that ships instead of a smaller one
standing in for it, and the venue still never mentions it.
*/
type Boot[S Driven] func(
	ctx context.Context,
	thesis *types.Thesis,
	public websocket.Conn,
	private websocket.Conn,
	bus *runtime.Workspace,
) S

/*
WithStack runs one test against the whole system wired to a simulated venue,
and tears both down when the test finishes.

The Thesis the system observes into is created here and reaches the test body
through the assembled system, because a Thesis handed to a test separately from
the system that fills it is one a test can read before anything has written it.
*/
func WithStack[S Driven](
	t *testing.T,
	symbols []*tes.Symbol,
	boot Boot[S],
	f func(*Market, S),
) func() {
	return WithMarket(t, symbols, drive(t, boot, f))
}

/*
WithOrders is WithStack against the simulated private REST transport, for tests
that submit orders rather than only observing.
*/
func WithOrders[S Driven](
	t *testing.T,
	symbols []*tes.Symbol,
	boot Boot[S],
	f func(*Market, S),
) func() {
	return WithFixtureOrders(t, symbols, drive(t, boot, f))
}

/*
drive boots the caller's system against a running venue, points the venue at it
so it can observe what the system carries, and hands both to the test body.
*/
func drive[S Driven](
	t *testing.T,
	boot Boot[S],
	f func(*Market, S),
) func(*Market) {
	return func(market *Market) {
		var absent S
		previousConfig := system.Cfg
		system.Cfg = system.NewConfig()
		defer func() {
			system.Cfg = previousConfig
		}()
		previousDataPath := viper.GetString("system.data_path")
		viper.Set("system.data_path", t.TempDir())
		defer viper.Set("system.data_path", previousDataPath)
		previousUIAddr := viper.Get("ui.addr")
		viper.Set("ui.addr", "127.0.0.1:0")
		defer viper.Set("ui.addr", previousUIAddr)
		previousBatch := viper.Get("market.subscribe.batch")
		previousPace := viper.Get("market.subscribe.pace")
		previousDepth := viper.Get("market.l3_depth")
		bookDepth := 0

		for _, symbol := range market.Symbols {
			bookDepth = max(bookDepth, symbol.BookDepthLevels)
		}

		viper.Set("market.subscribe.batch", len(market.Symbols))
		viper.Set("market.subscribe.pace", 0)
		viper.Set("market.l3_depth", bookDepth)
		defer viper.Set("market.subscribe.batch", previousBatch)
		defer viper.Set("market.subscribe.pace", previousPace)
		defer viper.Set("market.l3_depth", previousDepth)

		public, private := market.Feeds()
		bus := runtime.NewWorkspace(nil)
		thesis := types.NewThesis(t.Context())
		system := boot(t.Context(), thesis, public, private, bus)

		if system == absent {
			t.Fatal("tests: boot produced no system")
			return
		}

		market.WithStagedReplay(bus, system.Error)

		runErr := make(chan error, 1)

		go func() {
			runErr <- system.Run()
		}()

		defer func() {
			if err := system.Close(); err != nil {
				t.Errorf("tests: close system: %v", err)
			}

			if err := <-runErr; err != nil {
				t.Errorf("tests: run system: %v", err)
			}
		}()

		market.Drive(system)
		f(market, system)
	}
}
