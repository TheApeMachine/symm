package tests

import (
	"context"
	"fmt"
	"iter"
	"time"

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
type MarketAction = marketsignal.Action
type MarketStep = marketsignal.Step
type MarketActionKind = marketsignal.ActionKind

const (
	MarketStateBaseline = marketsignal.Baseline
	MarketStateFastPump = marketsignal.FastPump
	MarketStateSlowPump = marketsignal.SlowPump
	MarketStateFastDump = marketsignal.FastDump
	MarketStateSlowDump = marketsignal.SlowDump
	MarketMoveMid       = marketsignal.MoveMid
	MarketTrade         = marketsignal.Trade
	MarketAdd           = marketsignal.Add
	MarketCancel        = marketsignal.Cancel
	MarketRefill        = marketsignal.Refill
	MarketWidenSpread   = marketsignal.WidenSpread
)

/*
MarketOptions supplies the deterministic start time shared by every generated
wire source while preserving the one-line default constructor used by tests.
*/
type MarketOptions struct {
	Start time.Time
}

var defaultMarketStart = time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)

const (
	defaultPaperQuoteBalance = 80595.4943
	defaultPaperFeeRate      = 0.0026
)

/*
Market ranges ready fixture payloads into the fake Kraken connections.
*/
type Market struct {
	ctx     context.Context
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
	check  *Validator
}

/*
NewMarket creates fixture-fed connections for exactly symbolCount symbols.
*/
func NewMarket(
	ctx context.Context,
	symbolCount int,
	options ...MarketOptions,
) *Market {
	if symbolCount < 1 {
		panic(errnie.Err(errnie.Validation, "tests: symbol count must be positive", nil))
	}

	if len(options) > 1 {
		panic(errnie.Err(errnie.Validation, "tests: one market options value supported", nil))
	}

	start := defaultMarketStart

	if len(options) == 1 {
		if options[0].Start.IsZero() {
			panic(errnie.Err(errnie.Validation, "tests: market start time required", nil))
		}

		start = options[0].Start
	}

	ctx, cancel := context.WithCancel(ctx)
	symbols := make([]string, symbolCount)

	for index := range symbolCount {
		symbols[index] = fmt.Sprintf("SIM%d/USD", index+1)
	}

	signal := marketsignal.NewAt(symbols, start)
	market := &Market{
		ctx:     ctx,
		cancel:  cancel,
		Public:  mockapi.NewConn(symbols...),
		Private: mockapi.NewConn(symbols...),
		Paper:   mockapi.NewConn(symbols...),
		Level3:  mockapi.NewConn(symbols...),
		Symbols: symbols,
		State:   MarketStateBaseline,
		signal:  signal,
		ticker:  tickerfixture.NewMarket(symbols, signal),
		trade:   tradefixture.NewMarket(symbols, signal),
		book:    bookfixture.NewMarket(symbols, signal),
		level3:  level3fixture.NewMarket(symbols, signal),
		check:   NewValidator(),
	}
	market.configure()

	return market
}

/*
Now returns the event time visible in the currently emitted market state.
*/
func (market *Market) Now() time.Time {
	return market.signal.Now()
}

/*
Bootstrap installs exactly one current-state snapshot per subscribed market
channel without advancing time or warming signal history.
*/
func (market *Market) Bootstrap() error {
	market.signal.Bootstrap()
	payloads, err := market.frames()

	if err != nil {
		return err
	}

	if err := market.check.Validate(payloads); err != nil {
		return errnie.Err(
			errnie.Validation,
			"tests: invalid bootstrap market: "+err.Error(),
			err,
		)
	}

	market.Public.Respond("ticker", payloads.ticker)
	market.Public.Respond("trade", payloads.trade)
	market.Public.Respond("book", payloads.book)
	market.Level3.Respond("level3", payloads.level3)
	market.Public.RespondCurrent("ticker", func() []byte { return market.current().ticker })
	market.Public.RespondCurrent("trade", func() []byte { return market.current().trade })
	market.Public.RespondCurrent("book", func() []byte { return market.current().book })
	market.Level3.RespondCurrent("level3", func() []byte { return market.current().level3 })
	return nil
}

/*
Warmup explicitly replays a quiet market one observation at a time.
*/
func (market *Market) Warmup(afterStep func() error) error {
	return market.Transition(MarketStateBaseline, afterStep)
}

/*
Transition communicates one semantic scenario to every fixture, validates that
their tapes remain aligned, and invokes afterStep after each coherent emission.
*/
func (market *Market) Transition(
	state MarketState,
	afterStep func() error,
) error {
	if afterStep == nil {
		return errnie.Err(errnie.Validation, "tests: transition step callback required", nil)
	}

	steps, err := market.signal.Scenario(state)

	if err != nil {
		return errnie.Err(errnie.Validation, "tests: invalid market transition", err)
	}

	market.State = state

	for _, step := range steps {
		if err := market.Apply(step, afterStep); err != nil {
			return err
		}
	}

	return nil
}

/*
Apply advances one explicit economic step, validates all derived feeds, emits
them in causal order, and lets the production graph consume that observation.
*/
func (market *Market) Apply(
	step marketsignal.Step,
	afterStep func() error,
) error {
	if err := market.ctx.Err(); err != nil {
		return errnie.Err(errnie.Internal, "tests: market closed", err)
	}

	if afterStep == nil {
		return errnie.Err(errnie.Validation, "tests: step callback required", nil)
	}

	if err := market.signal.Apply(step); err != nil {
		return errnie.Err(errnie.Validation, "tests: apply market step", err)
	}

	payloads, err := market.frames()

	if err != nil {
		return err
	}

	if err := market.check.Validate(payloads); err != nil {
		return errnie.Err(
			errnie.Validation,
			"tests: invalid market step: "+err.Error(),
			err,
		)
	}

	if err := market.Level3.Queue("level3", payloads.level3); err != nil {
		return errnie.Err(errnie.IO, "tests: queue level3 frame", err)
	}

	for _, frame := range []struct {
		channel string
		payload []byte
	}{
		{"book", payloads.book},
		{"trade", payloads.trade},
		{"ticker", payloads.ticker},
	} {
		if err := market.Public.Queue(frame.channel, frame.payload); err != nil {
			return errnie.Err(errnie.IO, "tests: queue "+frame.channel+" frame", err)
		}
	}

	if err := market.Level3.Drain(); err != nil {
		return errnie.Err(errnie.IO, "tests: drain level3 frames", err)
	}

	if err := market.Public.Drain(); err != nil {
		return errnie.Err(errnie.IO, "tests: drain public frames", err)
	}

	if err := market.Paper.MatchPaper(); err != nil {
		return errnie.Err(errnie.Internal, "tests: match resting paper orders", err)
	}

	if err := market.Paper.Drain(); err != nil {
		return errnie.Err(errnie.IO, "tests: drain private frames", err)
	}

	if err := afterStep(); err != nil {
		return errnie.Err(errnie.Internal, "tests: market step failed", err)
	}

	if err := market.Paper.Drain(); err != nil {
		return errnie.Err(errnie.IO, "tests: drain order responses", err)
	}

	return nil
}

/*
configure assigns fixture streams to the Kraken subscription and REST routes.
*/
func (market *Market) configure() {
	err := market.Paper.EnablePaper(mockapi.PaperOptions{
		Quote: func(symbol string) (float64, float64, bool) {
			quote, exists := market.signal.Quote(symbol)
			return quote.Bid, quote.Ask, exists
		},
		Now: market.signal.Now,
		Balances: map[string]float64{
			"USD": defaultPaperQuoteBalance,
		},
		FeeRate: defaultPaperFeeRate,
	})

	if err != nil {
		panic(err)
	}

	for payload := range instrumentfixture.NewMarket(
		market.Symbols,
		marketsignal.PriceIncrement,
	).Generate() {
		market.Public.Respond("instrument", payload)
	}

	for payload := range balancesfixture.NewMarket("USD").Generate() {
		market.Paper.Respond("balances", payload)
	}

	for payload := range tradevolumefixture.NewMarket(market.Symbols).Generate() {
		market.Private.RespondPost("/0/private/TradeVolume", payload)
	}
}

type frameSet struct {
	ticker []byte
	trade  []byte
	book   []byte
	level3 []byte
}

/*
frames reads one bootstrap frame from each fixture and rejects missing or extra data.
*/
func (market *Market) frames() (frameSet, error) {
	return market.read(market.ticker, market.trade, market.book, market.level3)
}

/*
current renders one full snapshot of the present authoritative state for a new
or reconnecting subscription without replaying prior updates.
*/
func (market *Market) current() frameSet {
	frames, err := market.read(
		tickerfixture.NewMarket(market.Symbols, market.signal),
		tradefixture.NewMarket(market.Symbols, market.signal),
		bookfixture.NewMarket(market.Symbols, market.signal),
		level3fixture.NewMarket(market.Symbols, market.signal),
	)

	if err != nil {
		panic(err)
	}

	return frames
}

/*
read consumes exactly one ready payload from each coordinated fixture.
*/
func (market *Market) read(
	ticker *tickerfixture.Fixture,
	trade *tradefixture.Fixture,
	book *bookfixture.Fixture,
	level3 *level3fixture.Fixture,
) (frameSet, error) {
	tickerNext, tickerStop := iter.Pull(ticker.Generate())
	tradeNext, tradeStop := iter.Pull(trade.Generate())
	bookNext, bookStop := iter.Pull(book.Generate())
	level3Next, level3Stop := iter.Pull(level3.Generate())
	defer tickerStop()
	defer tradeStop()
	defer bookStop()
	defer level3Stop()

	tickerPayload, tickerMore := tickerNext()
	tradePayload, tradeMore := tradeNext()
	bookPayload, bookMore := bookNext()
	level3Payload, level3More := level3Next()

	if !tickerMore || !tradeMore || !bookMore || !level3More {
		return frameSet{}, errnie.Err(errnie.Validation, "tests: bootstrap frame missing", nil)
	}

	_, tickerExtra := tickerNext()
	_, tradeExtra := tradeNext()
	_, bookExtra := bookNext()
	_, level3Extra := level3Next()

	if tickerExtra || tradeExtra || bookExtra || level3Extra {
		return frameSet{}, errnie.Err(errnie.Validation, "tests: fixture frame count differs", nil)
	}

	return frameSet{
		ticker: tickerPayload,
		trade:  tradePayload,
		book:   bookPayload,
		level3: level3Payload,
	}, nil
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
