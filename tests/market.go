package tests

import (
	"context"
	"fmt"
	"os"
	"slices"
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
	// tickInterval paces the simulated feed so each Tick is one the stack
	// actually processed. It is the shortest gap at which the analyzer keeps
	// up with the generator on the machines this suite runs on.
	tickInterval = 10 * time.Millisecond
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
	capture      *thesisCapture
	generators   map[string]*signal.Generator
	latest       map[string]testtypes.Sample
	decisions    []types.Decision
	measurements map[string][]*types.Measurement
	filled       map[string]struct{}
	autoFill     bool
	primed       bool
	decisionMu   sync.Mutex
	sampleMu     sync.RWMutex
}

/*
retainMeasurements copies the current cycle's signal readings aside so tests
can inspect them after a completed decision closes the Thesis evidence cycle.
*/
func (market *Market) retainMeasurements() {
	market.decisionMu.Lock()
	defer market.decisionMu.Unlock()

	if market.measurements == nil {
		market.measurements = map[string][]*types.Measurement{}
	}

	market.Thesis.Measurements.Range(func(key, value any) bool {
		source, ok := key.(types.SourceType)

		if !ok {
			return true
		}

		if rows, ok := value.([]*types.Measurement); ok && len(rows) > 0 {
			market.measurements[string(source)] = rows
		}

		return true
	})
}

/*
Decisions returns every decision the planner has published so far. Collecting
the emitted copy preserves completed decision sets after their Thesis cycle has
closed.
*/
func (market *Market) Decisions() []types.Decision {
	market.decisionMu.Lock()
	defer market.decisionMu.Unlock()

	return slices.Clone(market.decisions)
}

/*
collectDecisions drains the planner's decision stream for the life of the
market.
*/
func (market *Market) collectDecisions() {
	if market.system == nil || market.system.Planner == nil {
		return
	}

	subscription := market.system.Planner.Subscribe(
		"decisions", types.NewSubscription[any](),
	)

	go func() {
		for {
			select {
			case <-market.ctx.Done():
				return
			case published := <-subscription.Channel:
				decisions, ok := published.([]types.Decision)

				if !ok {
					continue
				}

				market.decisionMu.Lock()
				market.decisions = append(market.decisions, decisions...)
				market.decisionMu.Unlock()
			}
		}
	}()
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

	market.collectDecisions()
	market.Desk = market.system.Desk
	market.Planner = market.system.Planner
	market.Analyzer = market.system.Analyzer
	market.capture = newThesisCapture(market.Planner)
	market.Analyzer.AttachEvaluator(market.capture)

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
	market.capture.Enable(symbol)
	generator.SetState(state, testtypes.MomentumMap[state])

	for generator.PrecursorPending() {
		market.Tick()
	}

	return market.waitForTransitionThesis(symbol)
}

/*
TransitionThesis returns the last analyzed precursor Thesis captured before the
real planner consumed it.
*/
func (market *Market) TransitionThesis() *types.Thesis {
	return market.capture.Snapshot()
}

func (market *Market) waitForTransitionThesis(symbol string) error {
	market.sampleMu.RLock()
	published := market.latest[symbol]
	market.sampleMu.RUnlock()

	actorCapacity := viper.GetInt("system.actor.buffer")

	if actorCapacity < 1 {
		actorCapacity = 64
	}

	for range actorCapacity {
		snapshot := market.TransitionThesis()

		if snapshot != nil {
			latest, found := snapshot.LatestTicker(symbol)
			measuredAt := latestMeasurementAt(
				snapshot, types.SourcePumpDump, symbol,
			)

			if found && !latest.Timestamp.Before(published.Timestamp) &&
				!measuredAt.Before(published.Timestamp) {
				return nil
			}
		}

		time.Sleep(tickInterval)
	}

	return fmt.Errorf("market: precursor Thesis did not reach %s", published.Timestamp)
}

func latestMeasurementAt(
	thesis *types.Thesis,
	source types.SourceType,
	symbol string,
) time.Time {
	stored, found := thesis.Measurements.Load(source)

	if !found {
		return time.Time{}
	}

	rows, ok := stored.([]*types.Measurement)

	if !ok {
		return time.Time{}
	}

	var latest time.Time

	for _, measurement := range rows {
		if measurement != nil && measurement.Symbol == symbol &&
			measurement.At.After(latest) {
			latest = measurement.At
		}
	}

	return latest
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

	/*
		Answer pending orders before the processing pause so the execution and
		this tick's market frames can both reach the desk before the next tick.
		Publishing is asynchronous, so without pausing here the loop feeding the
		market outruns the stages consuming it and a test measures a pipeline
		that never got to run.
	*/
	market.fillPending()
	time.Sleep(tickInterval)
	market.retainMeasurements()
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

		market.Private.Publish("executions", executionfixture.Frame(fill))
	}
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
WithMarket boots a full symm stack against a simulated market, attaches the
market data feeds, and tears everything down when the test finishes. The test
body receives a ready market and asserts against market.Thesis.
*/
func WithMarket(t *testing.T, symbols []*testtypes.Symbol, f func(*Market)) func() {
	return func() {
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		So(market.system != nil, ShouldBeTrue)

		Reset(func() {
			market.Close()
		})

		f(market)
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
