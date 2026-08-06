package tests

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/tests/fixtures/book"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	"github.com/theapemachine/symm/tests/fixtures/level3"
	"github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/tests/fixtures/trade"
	"github.com/theapemachine/symm/tests/signal"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
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

	/*
		passCeiling is how long the feed will wait for the stack to analyze what
		it published before calling the stack stopped rather than slow.

		It is deliberately far longer than a pass costs. How long one takes is a
		property of the symbol count and of the stages themselves, so a ceiling
		tight enough to be a performance assertion would fail on a busy machine
		and say nothing about the market. This only has to separate slow from
		stopped, and the wait ends the moment the work lands.
	*/
	passCeiling = 30 * time.Second
)

/*
Market ranges ready fixture payloads into the fake Kraken connections.
*/
type Market struct {
	ctx          context.Context
	cancel       context.CancelFunc
	Public       *Conn
	Private      *Conn
	Level3       *Conn
	Symbols      []*testtypes.Symbol
	State        testtypes.MarketState
	system       *cmd.System
	public       *websocket.Live
	private      *websocket.Live
	Thesis       *types.Thesis
	Desk         *broker.Desk
	Planner      *strategy.Planner
	Analyzer     *logic.Analyzer
	generators   map[string]*signal.Generator
	latest       map[string]testtypes.Sample
	previous     map[string]testtypes.Sample
	decisions    []types.Decision
	measurements map[string][]*types.Measurement
	filled       map[string]struct{}
	autoFill     bool
	primed       bool
	decisionMu   sync.Mutex
	sampleMu     sync.RWMutex
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
		Thesis:     types.NewThesis(nil),
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

	market.system = cmd.Boot(
		ctx, market.Thesis, market.public, market.private, nil,
	)

	market.Desk = market.system.Desk
	market.Planner = market.system.Planner
	market.Analyzer = market.system.Analyzer
	pairs := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		pairs = append(pairs, symbol.Pair)
	}

	return market
}

/*
Measurements flattens the measurements the signals have written, so assertions
can inspect them as an ordinary map. The harness copy also keeps completed
cycles available after decision emission closes the live Thesis cycle.
*/
func (market *Market) Measurements() map[string][]*types.Measurement {
	measurements := map[string][]*types.Measurement{}

	market.decisionMu.Lock()

	for source, rows := range market.measurements {
		measurements[source] = rows
	}

	market.decisionMu.Unlock()

	market.Thesis.Measurements.Range(func(key, value any) bool {
		source, ok := key.(types.SourceType)

		if !ok {
			return true
		}

		if found, ok := value.([]*types.Measurement); ok {
			measurements[string(source)] = found
		}

		return true
	})

	return measurements
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
	for range flattenCeilingTicks {
		if !market.holds(symbol) {
			return nil
		}

		market.Tick()
	}

	return fmt.Errorf(
		"market: %s was still held after %d ticks", symbol, flattenCeilingTicks,
	)
}

/*
holds answers whether the desk still carries an unclosed lot for one symbol.
*/
func (market *Market) holds(symbol string) bool {
	if market.Desk == nil {
		return false
	}

	for position := range market.Desk.Positions() {
		if position.Holding == nil || position.Holding.Symbol != symbol {
			continue
		}

		if position.Holding.Status != types.CLOSED {
			return true
		}
	}

	return false
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

	market.settleTick()
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

func (market *Market) settleTick() {
	// Answer pending orders before the processing pause so the execution and
	// this tick's market frames can both reach the desk before the next tick.
	market.fillPending()
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

/*
fillPending settles any order the desk has submitted but not yet seen filled.

A live venue answers a market order with an execution on the private channel,
and until that arrives a position stays PENDING and never becomes something the
strategy can manage or close. Nothing else in the harness plays the venue, so
without this the desk accumulates orders that never become positions.
*/
func (market *Market) fillPending() {
	if market.Desk == nil || !market.autoFill {
		return
	}

	for position := range market.Desk.Positions() {
		if position.Status != types.PENDING || position.Holding == nil ||
			position.EntryOrder == nil || position.EntryOrderResult == nil {
			continue
		}

		fill := executionfixture.BuyFill()
		order := position.EntryOrder
		result := position.EntryOrderResult

		if position.ExitOrderResult != nil {
			fill = executionfixture.ExitFill()
			order = position.ExitOrder
			result = position.ExitOrderResult
		}

		if order == nil || result == nil || len(result.ID) == 0 {
			continue
		}

		fill.OrderID = result.ID[0]

		if _, done := market.filled[order.ClOrdId]; done {
			continue
		}

		market.filled[order.ClOrdId] = struct{}{}
		fill.ClientOrderID = order.ClOrdId
		fill.Symbol = position.Holding.Symbol
		fill.CumQty = order.Volume
		fill.LastQty = fill.CumQty

		if err := market.priceFill(&fill, position.Holding.Symbol); err != nil {
			panic(err)
		}

		market.Private.Publish("executions", executionfixture.Frame(fill))
	}
}

/*
priceFill answers an order at the price the simulated venue would actually have
executed it at.

The fill options this borrows are broker unit-test constants — a buy averaging
105 and a sell averaging 110 — which describe the fixture they were written for
and nothing about this market. Left in place they book every entry and exit at
those two prices however the symbol is quoted, so a position opened at sixty
thousand records a four percent gain it never made, and every realised return
measured here would be that arithmetic rather than the strategy's.

A taker buy lifts the ask and a taker sell hits the bid, both at the touch this
tick published, and the fee is the same schedule the entry was priced against.
That is what makes a realised profit here a claim about the decisions rather
than about the fixture.
*/
func (market *Market) priceFill(fill *executionfixture.Options, symbol string) error {
	market.sampleMu.RLock()
	sample, known := market.latest[symbol]
	market.sampleMu.RUnlock()

	if !known {
		return fmt.Errorf("market: cannot price a fill for unpublished symbol %q", symbol)
	}

	price := sample.Ask

	if fill.Side == "sell" {
		price = sample.Bid
	}

	if price <= 0 {
		return fmt.Errorf("market: %s has no executable touch to fill against", symbol)
	}

	quantity, err := strconv.ParseFloat(fill.CumQty, 64)

	if err != nil || quantity <= 0 {
		return fmt.Errorf("market: fill for %s carries no executable quantity", symbol)
	}

	rate, err := market.Desk.Price().Fee(symbol)

	if err != nil {
		return fmt.Errorf("market: no fee schedule to charge %s against: %w", symbol, err)
	}

	cost := price * quantity

	fill.LastPrice = strconv.FormatFloat(price, 'f', -1, 64)
	fill.AvgPrice = fill.LastPrice
	fill.Cost = strconv.FormatFloat(cost, 'f', -1, 64)
	fill.CumCost = fill.Cost
	fill.FeeUsdEquiv = strconv.FormatFloat(cost*rate.Float64(), 'f', -1, 64)
	fill.Timestamp = sample.Timestamp.UTC().Format(time.RFC3339Nano)

	return nil
}

func (market *Market) Close() {
	if market.system != nil {
		errnie.Error(market.system.Close())
		market.system = nil
	}

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
	f func(*Market, *types.Thesis),
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
WithMarket boots a full symm stack against a simulated market, attaches the
market data feeds, and tears everything down when the test finishes. The test
body receives a ready market and asserts against market.Thesis.
*/
func WithMarket(t *testing.T, symbols []*testtypes.Symbol, f func(*Market, *types.Thesis)) func() {
	return func() {
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		So(market.system != nil, ShouldBeTrue)

		Reset(func() {
			market.Close()
		})

		f(market, market.system.Thesis)
	}
}

/*
EntryRisk derives the stop geometry an entry for this symbol would be sized
under, from the live simulated book.

Tests that submit entries need it because the desk refuses an entry without one:
the quantity on a decision was solved against a particular risk distance, and a
lot fitted with some other distance after the fact is carrying a loss nobody
budgeted. A bare decision would exercise a path production does not have.
*/
func EntryRisk(market *Market, symbol string) types.RiskPlan {
	if market == nil || market.Desk == nil {
		return types.RiskPlan{}
	}

	pair, err := market.Desk.Instrument().Pair(symbol)

	if err != nil {
		return types.RiskPlan{}
	}

	return market.Desk.Price().RiskPlan(pair)
}
