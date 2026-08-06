package tests

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/fixtures/book"
	"github.com/theapemachine/symm/tests/fixtures/level3"
	"github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/tests/fixtures/trade"
	"github.com/theapemachine/symm/tests/signal"
	testtypes "github.com/theapemachine/symm/tests/types"
)

const (
	// tickInterval is how often the feed re-checks whether the stack has caught
	// up with it. Whether it has is observed rather than assumed, so this only
	// decides the granularity of the wait.
	tickInterval = 10 * time.Millisecond

	/*
		flattenCeilingTicks bounds how long a lot may take to close before the
		harness calls it stuck. It is not a wait anything is expected to reach:
		a regulator that has armed closes on the reversal within the transition
		the reversal takes, so this only has to be longer than a regime change
		to distinguish a slow exit from one that is never coming.
	*/
	flattenCeilingTicks = 512
)

/*
Market ranges ready fixture payloads into the fake Kraken connections.
*/
type Market struct {
	ctx        context.Context
	cancel     context.CancelFunc
	Public     *Conn
	Private    *Conn
	Level3     *Conn
	Symbols    []*testtypes.Symbol
	State      testtypes.MarketState
	public     *websocket.Live
	private    *websocket.Live
	generators map[string]*signal.Generator
	latest     map[string]testtypes.Sample
	previous   map[string]testtypes.Sample
	filled     map[string]struct{}
	stack      Stack
	autoFill   bool
	primed     bool
	sampleMu   sync.RWMutex
}

/*
Drive points the venue at the system consuming its frames.

A venue that publishes into nothing can only run for a fixed number of ticks,
which turns every wait into a number somebody guessed. Given the system, it can
run until what it is waiting for has actually happened.
*/
func (market *Market) Drive(stack Stack) *Market {
	market.stack = stack

	return market
}

/*
NewMarket creates a simulated market with the given number of symbols.
It replaces the production Kraken API WebSockets and REST routes with
in-memory fixtures that emit deterministic events for testing.
*/
func NewMarket(
	ctx context.Context,
	symbols []*testtypes.Symbol,
) *Market {
	return newMarket(ctx, symbols, nil)
}

/*
NewMarketWithAccount boots the fixture stack from an existing wallet and its
account fills, matching a process restart against an account that already owns
inventory.
*/
func NewMarketWithAccount(
	ctx context.Context,
	symbols []*testtypes.Symbol,
	balances map[string]string,
	trades map[string]spot.Trade,
) *Market {
	return newMarket(ctx, symbols, func(private *Conn) {
		private.ConfigureAccount(balances, trades)
	})
}

func newMarket(
	ctx context.Context,
	symbols []*testtypes.Symbol,
	configureAccount func(*Conn),
) *Market {
	tradingModel := viper.GetString("trading.model")
	viper.Set("trading.model", "real")
	defer viper.Set("trading.model", tradingModel)

	ctx, cancel := context.WithCancel(ctx)

	market := &Market{
		ctx:        ctx,
		cancel:     cancel,
		Public:     NewConn(ctx),
		Private:    NewConn(ctx),
		Level3:     NewConn(ctx),
		Symbols:    symbols,
		State:      testtypes.Baseline,
		generators: make(map[string]*signal.Generator, len(symbols)),
		latest:     make(map[string]testtypes.Sample, len(symbols)),
		previous:   make(map[string]testtypes.Sample, len(symbols)),
		filled:     make(map[string]struct{}),
	}

	for _, symbol := range symbols {
		generator := signal.NewGenerator(
			symbol.Pair, symbol.StartPrice, symbol.Seed,
		)

		market.generators[symbol.Pair] = generator
	}

	market.Public.Configure(symbols)
	market.Private.Configure(symbols)
	market.Level3.Configure(symbols)

	if configureAccount != nil {
		configureAccount(market.Private)
	}

	// The fixture Conn owns the in-memory transport; Live wraps its client
	// so that production parsing, routing, and book handling all run
	// unchanged against the simulated frames.
	market.public = websocket.NewWithClient(
		ctx, nil, false,
		websocket.PublicWebSocketURL, market.Public.Client(),
	)

	market.private = websocket.NewWithClient(
		ctx, nil, true,
		websocket.PrivateWebSocketURL, market.Private.Client(),
	)

	// SubL3 opens child connections for the Level3 book. Point them at the
	// fixture's Level3 transport so the books are built from simulated
	// depth rather than a real Kraken endpoint.
	market.private.Level3Client = market.Level3.Client

	return market
}

/*
Transition moves one symbol into another market state. This should not be an
instant regime shift; the selected generator takes a realistic number of ticks
to move from one state into another.
*/
func (market *Market) Transition(
	symbol string,
	state testtypes.MarketState,
) error {
	generator, ok := market.generators[symbol]

	if !ok {
		return fmt.Errorf("market: cannot transition unknown symbol %q", symbol)
	}

	if !market.primed {
		baselineSamples := viper.GetInt("signals.pumpdump.baselineCapacity")

		if baselineSamples <= 0 {
			return fmt.Errorf("market: positive empirical baseline capacity required")
		}

		for range baselineSamples {
			market.Tick()
		}

		market.primed = true
	}

	market.State = state
	generator.SetState(state, testtypes.MomentumMap[state])

	for generator.PrecursorPending() {
		market.Tick()
	}

	return nil
}

/*
LastSample returns the venue state this feed most recently published for one
symbol, which is the book any fill answered at that moment was struck against.
*/
func (market *Market) LastSample(symbol string) (testtypes.Sample, bool) {
	market.sampleMu.RLock()
	defer market.sampleMu.RUnlock()

	sample, known := market.latest[symbol]

	return sample, known
}

/*
Express runs a transitioned regime through the discontinuous event it was
configured with, until the burst that opens it has fully decayed.

Transition stops at the precursor because that is the exact moment an entry has
to be judged on. This is the other end of the same move: the generator knows
when its ignition has printed and when the continuation has decayed to nothing,
so a test observing what the whole regime produced runs to that rather than to a
tick count somebody picked.
*/
func (market *Market) Express(symbol string) error {
	generator, ok := market.generators[symbol]

	if !ok {
		return fmt.Errorf("market: cannot express unknown symbol %q", symbol)
	}

	for !generator.IgnitionSpent() {
		market.Tick()
	}

	return nil
}

/*
Flatten runs the market on until the desk carries no open lot for the symbol.

How long a position takes to close is a property of the geometry its regulator
set on the way in and of the prices that followed, so it is observed rather than
assumed. The ceiling is what makes a lot that never closes a failing test that
names the position instead of a hanging one.
*/
func (market *Market) Flatten(symbol string) error {
	if market.stack == nil {
		return fmt.Errorf(
			"market: %s cannot be run flat without a driven stack", symbol,
		)
	}

	for range flattenCeilingTicks {
		if market.stack.Holding(symbol) == 0 {
			return nil
		}

		market.Tick()
	}

	return fmt.Errorf(
		"market: %s was still held after %d ticks", symbol, flattenCeilingTicks,
	)
}

/*
Tick the market. Each symbol advances one generator step, and that coherent
sample is rendered into every channel before the frames are published exactly
as if they had arrived over real WebSockets. This does not give you any result,
because the result should come from the code that is currently being tested.
*/
func (market *Market) Tick() {
	for _, symbol := range market.Symbols {
		generator := market.generators[symbol.Pair]
		_ = market.publish(generator)
	}
}

/*
publish samples one coherent venue state and sends it through every real
WebSocket channel consumed by the production stack.
*/
func (market *Market) publish(generator *signal.Generator) testtypes.Sample {
	sample := generator.Step()
	market.sampleMu.Lock()

	if known, ok := market.latest[sample.Symbol]; ok {
		market.previous[sample.Symbol] = known
	}

	market.latest[sample.Symbol] = sample
	market.sampleMu.Unlock()

	market.Public.Publish(
		"ticker",
		ticker.NewFixture(ticker.UPDATE, 1, generator).Render(sample),
	)

	market.Public.Publish(
		"book",
		book.NewFixture(book.UPDATE, 1, generator).Render(sample),
	)

	market.Level3.Publish(
		"level3",
		level3.NewFixture(level3.SNAPSHOT, 1, generator).Render(sample),
	)

	market.waitForBook(sample)

	market.Public.Publish(
		"trade",
		trade.NewFixture(trade.UPDATE, 1, generator).Render(sample),
	)

	return sample
}

/*
waitForBook preserves venue causality between a depth update and a trade at
that depth. Conn confirms frame dispatch, while the production book callback
may still be applying it; the simulated trade must not overtake that work.
*/
func (market *Market) waitForBook(sample testtypes.Sample) {
	actorCapacity := viper.GetInt("system.actor.buffer")
	var observedBid float64
	var observedAsk float64
	var bidLevels int
	var askLevels int

	if actorCapacity < 1 {
		actorCapacity = 64
	}

	for range actorCapacity {
		liveBook := market.private.Book(sample.Symbol)

		if liveBook != nil {
			bidLevels = len(liveBook.Bids.Levels)
			askLevels = len(liveBook.Asks.Levels)
			bid := liveBook.BestBid()
			ask := liveBook.BestAsk()

			if bid != nil {
				observedBid = bid.Price.Float64()
			}

			if ask != nil {
				observedAsk = ask.Price.Float64()
			}

			if bid != nil && ask != nil &&
				bid.Price.Float64() == sample.Bid &&
				ask.Price.Float64() == sample.Ask &&
				!bid.Timestamp.Before(sample.Timestamp) &&
				!ask.Timestamp.Before(sample.Timestamp) {
				return
			}
		}

		time.Sleep(tickInterval)
	}

	panic(fmt.Errorf(
		"market: live book did not reach %s at %s: want %g/%g, got %g/%g across %d/%d levels",
		sample.Symbol,
		sample.Timestamp,
		sample.Bid,
		sample.Ask,
		observedBid,
		observedAsk,
		bidLevels,
		askLevels,
	))
}

/*
WithAutoFill makes the market answer its own orders with fills.

A test that drives the strategy end to end needs positions to open without
staging every execution itself. Tests that publish their own execution frames
leave this off, so the harness never fills an order out from under them.
*/
func (market *Market) WithAutoFill() *Market {
	market.autoFill = true

	return market
}

func (market *Market) Close() {
	market.Public.Close()
	market.Private.Close()
	market.Level3.Close()
	market.cancel()
}

/*
WithFixtureOrders routes orders through the simulated private REST transport
for the duration of one market test, then restores the caller's configuration.
*/
func WithFixtureOrders(
	t *testing.T,
	symbols []*testtypes.Symbol,
	f func(*Market),
) func() {
	return func() {
		tradingModel := viper.GetString("trading.model")
		apiKey, hadAPIKey := os.LookupEnv("KRAKEN_API_KEY")
		apiSecret, hadAPISecret := os.LookupEnv("KRAKEN_API_SECRET")

		viper.Set("trading.model", "real")
		_ = os.Setenv("KRAKEN_API_KEY", "fixture-key")
		_ = os.Setenv("KRAKEN_API_SECRET", "Zml4dHVyZS1zZWNyZXQ=")

		defer func() {
			viper.Set("trading.model", tradingModel)

			if hadAPIKey {
				_ = os.Setenv("KRAKEN_API_KEY", apiKey)
			} else {
				_ = os.Unsetenv("KRAKEN_API_KEY")
			}

			if hadAPISecret {
				_ = os.Setenv("KRAKEN_API_SECRET", apiSecret)
			} else {
				_ = os.Unsetenv("KRAKEN_API_SECRET")
			}
		}()

		WithMarket(t, symbols, f)()
	}
}

/*
WithMarket runs a simulated venue for the duration of one test and tears its
connections down when the test finishes.

It boots nothing. The stack under test is wired by the caller against Feeds,
because a data provider that also assembled the system would have to import it,
and every package that owns a piece of the system could then no longer test
against a market. WithStack is the same venue with that wiring done, for the
tests that want the whole system rather than only its data.
*/
func WithMarket(t *testing.T, symbols []*testtypes.Symbol, f func(*Market)) func() {
	return func() {
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		Reset(func() {
			market.Close()
		})

		f(market)
	}
}

/*
Feeds returns the public and private connections the venue publishes through,
which is what a caller points its own stack at.
*/
func (market *Market) Feeds() (*websocket.Live, *websocket.Live) {
	return market.public, market.private
}
