package tests

import (
	"context"
	"fmt"
	"iter"

	"github.com/theapemachine/errnie"
	balancesfixture "github.com/theapemachine/symm/tests/fixtures/balances"
	bookfixture "github.com/theapemachine/symm/tests/fixtures/book"
	instrumentfixture "github.com/theapemachine/symm/tests/fixtures/instrument"
	level3fixture "github.com/theapemachine/symm/tests/fixtures/level3"
	marketsignal "github.com/theapemachine/symm/tests/fixtures/signal"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	tradefixture "github.com/theapemachine/symm/tests/fixtures/trade"
	tradevolumefixture "github.com/theapemachine/symm/tests/fixtures/tradevolume"
	"github.com/theapemachine/symm/tests/mockapi"
)

/*
MarketState names the deterministic state communicated to every fixture.
*/
type MarketState = marketsignal.State

const (
	MarketStateBaseline = marketsignal.Baseline
	MarketStateFastPump = marketsignal.FastPump
	MarketStateSlowPump = marketsignal.SlowPump
	MarketStateFastDump = marketsignal.FastDump
	MarketStateSlowDump = marketsignal.SlowDump
)

/*
Market ranges ready fixture payloads into the fake Kraken connections.
*/
type Market struct {
	cancel  context.CancelFunc
	Public  *mockapi.MockConn
	Private *mockapi.MockConn
	Paper   *mockapi.MockConn
	Level3  *mockapi.MockConn
	Symbols []string
	State   MarketState

	signal *marketsignal.Signal
	ticker *tickerfixture.Fixture
	trade  *tradefixture.Fixture
	book   *bookfixture.Fixture
	level3 *level3fixture.Fixture
}

/*
NewMarket creates fixture-fed connections for exactly symbolCount symbols.
*/
func NewMarket(ctx context.Context, symbolCount int) *Market {
	if symbolCount < 1 {
		panic(errnie.Err(errnie.Validation, "tests: symbol count must be positive", nil))
	}

	ctx, cancel := context.WithCancel(ctx)
	symbols := make([]string, symbolCount)

	for index := range symbolCount {
		symbols[index] = fmt.Sprintf("SIM%d/USD", index+1)
	}

	signal := marketsignal.New(symbols)
	market := &Market{
		cancel:  cancel,
		Public:  mockapi.NewConn(),
		Private: mockapi.NewConn(),
		Paper:   mockapi.NewConn(),
		Level3:  mockapi.NewConn(),
		Symbols: symbols,
		State:   MarketStateBaseline,
		signal:  signal,
		ticker:  tickerfixture.NewMarket(symbols, signal),
		trade:   tradefixture.NewMarket(symbols, signal),
		book:    bookfixture.NewMarket(symbols, signal),
		level3:  level3fixture.NewMarket(symbols, signal),
	}
	market.configure()

	return market
}

/*
Transition communicates a semantic state to each fixture and emits every frame.
*/
func (market *Market) Transition(state MarketState) {
	market.signal.Transition(state)
	tickerNext, tickerStop := iter.Pull(market.ticker.Generate())
	tradeNext, tradeStop := iter.Pull(market.trade.Generate())
	bookNext, bookStop := iter.Pull(market.book.Generate())
	level3Next, level3Stop := iter.Pull(market.level3.Generate())
	defer tickerStop()
	defer tradeStop()
	defer bookStop()
	defer level3Stop()

	for tickerPayload, more := tickerNext(); more; tickerPayload, more = tickerNext() {
		tradePayload, _ := tradeNext()
		bookPayload, _ := bookNext()
		level3Payload, _ := level3Next()
		market.Public.Emit("ticker", tickerPayload)
		market.Public.Emit("trade", tradePayload)
		market.Public.Emit("book", bookPayload)
		market.Level3.Emit("level3", level3Payload)
	}

	market.State = state
}

/*
configure assigns fixture streams to the Kraken subscription and REST routes.
*/
func (market *Market) configure() {
	for payload := range instrumentfixture.NewMarket(
		market.Symbols,
		marketsignal.PriceIncrement,
	).Generate() {
		market.Public.Respond("instrument", payload)
	}

	market.signal.Transition(MarketStateBaseline)

	for payload := range market.ticker.Generate() {
		market.Public.Respond("ticker", payload)
	}

	for payload := range market.trade.Generate() {
		market.Public.Respond("trade", payload)
	}

	for payload := range market.book.Generate() {
		market.Public.Respond("book", payload)
	}

	for payload := range market.level3.Generate() {
		market.Level3.Respond("level3", payload)
	}

	for payload := range balancesfixture.NewMarket("USD").Generate() {
		market.Paper.Respond("balances", payload)
	}

	for payload := range tradevolumefixture.NewMarket(market.Symbols).Generate() {
		market.Private.RespondPost("/0/private/TradeVolume", payload)
	}
}

/*
Close releases the four in-memory connections and simulated market context.
*/
func (market *Market) Close() {
	market.Public.Close()
	market.Private.Close()
	market.Paper.Close()
	market.Level3.Close()
	market.cancel()
}
