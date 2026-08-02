package tests

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/fixtures/book"
	"github.com/theapemachine/symm/tests/fixtures/level3"
	"github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/tests/fixtures/trade"
	"github.com/theapemachine/symm/tests/signal"
	testtypes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

const (
	defaultPaperQuoteBalance = 200.00
	defaultPaperMakerFee     = 0.0016
	defaultPaperTakerFee     = 0.0026
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
	system     *cmd.System
	public     *websocket.Live
	private    *websocket.Live
	Thesis     *types.Thesis
	generators map[string]*signal.Generator
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

	/*
		The fixture Conn owns the in-memory transport; Live wraps its client
		so that production parsing, routing, and book handling all run
		unchanged against the simulated frames.
	*/
	market.public = websocket.NewWithClient(
		ctx, nil, false,
		websocket.PublicWebSocketURL, market.Public.Client(),
	)

	market.private = websocket.NewWithClient(
		ctx, nil, true,
		websocket.PrivateWebSocketURL, market.Private.Client(),
	)

	/*
		SubL3 opens child connections for the Level3 book. Point them at the
		fixture's Level3 transport so the books are built from simulated
		depth rather than a real Kraken endpoint.
	*/
	market.private.Level3Client = market.Level3.Client

	market.system = cmd.Boot(
		ctx, market.Thesis, market.public, market.private,
	)

	return market
}

/*
Measurements flattens the measurements the signals have written onto the
current Thesis tick, so assertions can inspect them as an ordinary map.
*/
func (market *Market) Measurements() map[string][]*types.Measurement {
	measurements := map[string][]*types.Measurement{}

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
Tick the market. Each symbol advances one generator step per channel, and the
rendered frames are published into the connections exactly as if they had
arrived over a real WebSocket. This does not give you any result, because the
result should come from the code that is currently being tested.
*/
func (market *Market) Tick() {
	for _, symbol := range market.Symbols {
		generator := market.generators[symbol.Pair]

		for frame := range ticker.NewFixture(ticker.UPDATE, 1, generator).Generate() {
			market.Public.Publish("ticker", frame)
		}

		for frame := range book.NewFixture(book.UPDATE, 1, generator).Generate() {
			market.Public.Publish("book", frame)
		}

		for frame := range trade.NewFixture(trade.UPDATE, 1, generator).Generate() {
			market.Public.Publish("trade", frame)
		}

		for frame := range level3.NewFixture(level3.UPDATE, 1, generator).Generate() {
			market.Level3.Publish("level3", frame)
		}
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
