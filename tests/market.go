package tests

import (
	"context"
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
	generators   map[string]*signal.Generator
	decisions    []types.Decision
	measurements map[string][]*types.Measurement
	filled       map[string]struct{}
	autoFill     bool
	decisionMu   sync.Mutex
}

/*
retainMeasurements copies the current tick's signal readings aside before the
planner clears them.
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
Decisions returns every decision the planner has published so far.

The planner clears the thesis at the end of each evaluated tick, so the
decisions on it are gone by the time a test could read them. Collecting them
from the planner's own subscription instead is the only way to see the whole
run rather than whichever tick happened to be in flight.
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
	ctx, cancel := context.WithCancel(ctx)

	market := &Market{
		ctx:        ctx,
		cancel:     cancel,
		Public:     NewConn(ctx),
		Private:    NewConn(ctx),
		Level3:     NewConn(ctx),
		Symbols:    symbols,
		State:      testtypes.Baseline,
		Thesis:     types.NewThesis(),
		generators: make(map[string]*signal.Generator, len(symbols)),
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
		ctx, market.Thesis, market.public, market.private,
	)

	market.collectDecisions()
	market.Desk = market.system.Desk
	market.Planner = market.system.Planner
	market.Analyzer = market.system.Analyzer

	return market
}

/*
Measurements flattens the measurements the signals have written, so assertions
can inspect them as an ordinary map.

Readings seen on earlier ticks are retained, because the thesis is cleared once
a tick is evaluated and a signal that reported throughout the run would
otherwise appear only if it happened to publish on the very last one.
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
Transition into another market state. This should not be an instance regime
shift, instead it should take a realistic amount of ticks to move from one
state into another. This is meant to simulate an actual market regime shift.
*/
func (market *Market) Transition(state testtypes.MarketState) {
	market.State = state

	for _, generator := range market.generators {
		generator.SetState(state, testtypes.MomentumMap[state])
	}
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
		sample := generator.Step()

		market.Public.Publish(
			"ticker",
			ticker.NewFixture(ticker.UPDATE, 1, generator).Render(sample),
		)
		market.Public.Publish(
			"book",
			book.NewFixture(book.UPDATE, 1, generator).Render(sample),
		)
		market.Public.Publish(
			"trade",
			trade.NewFixture(trade.UPDATE, 1, generator).Render(sample),
		)
		market.Level3.Publish(
			"level3",
			level3.NewFixture(level3.UPDATE, 1, generator).Render(sample),
		)
	}

	/*
		Publishing is asynchronous, so without pausing here the loop feeding
		the market outruns the stages consuming it and a test measures a
		pipeline that never got to run. This paces the feed to something a
		live venue could plausibly deliver.
	*/
	time.Sleep(tickInterval)

	market.fillPending()
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

		So(market.system, ShouldNotBeNil)

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
