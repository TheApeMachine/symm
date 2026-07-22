package tests

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	bookfixture "github.com/theapemachine/symm/tests/fixtures/book"
	instrumentfixture "github.com/theapemachine/symm/tests/fixtures/instrument"
	level3fixture "github.com/theapemachine/symm/tests/fixtures/level3"
	marketsignal "github.com/theapemachine/symm/tests/fixtures/signal"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	tradefixture "github.com/theapemachine/symm/tests/fixtures/trade"
	tradevolumefixture "github.com/theapemachine/symm/tests/fixtures/tradevolume"
	"github.com/theapemachine/symm/tests/mockapi"
)

const (
	defaultPaperQuoteBalance = 80595.4943
	defaultPaperMakerFee     = 0.0016
	defaultPaperTakerFee     = 0.0026
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

	bootstrapped bool
	failed       error
	signal       *marketsignal.Signal
	ticker       *tickerfixture.Fixture
	trade        *tradefixture.Fixture
	book         *bookfixture.Fixture
	level3       *level3fixture.Fixture
	check        *Validator
}

/*
NewMarket creates fixture-fed connections for exactly symbolCount symbols.
*/
func NewMarket(ctx context.Context, symbolCount int, options ...MarketOptions) *Market {
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
	if err := market.ctx.Err(); err != nil {
		return errnie.Err(errnie.Internal, "tests: market closed", err)
	}

	if market.bootstrapped {
		return errnie.Err(errnie.Validation, "tests: market already bootstrapped", nil)
	}

	if market.failed != nil {
		return errnie.Err(errnie.Internal, "tests: market failed", market.failed)
	}

	market.signal.Bootstrap()
	payloads, err := market.read(
		market.ticker, market.trade, market.book, market.level3,
	)

	if err != nil {
		market.failed = err
		return err
	}

	if err := market.check.Validate(payloads); err != nil {
		market.failed = err
		return errnie.Err(
			errnie.Validation,
			"tests: invalid bootstrap market: "+err.Error(),
			err,
		)
	}

	market.Public.RespondCurrent("ticker", func() []byte { return market.current().ticker })
	market.Public.RespondCurrent("trade", func() []byte { return market.current().trade })
	market.Public.RespondCurrent("book", func() []byte { return market.current().book })
	market.Level3.RespondCurrent("level3", func() []byte { return market.current().level3 })
	market.bootstrapped = true
	return nil
}

/*
Warmup explicitly replays a quiet market one observation at a time.
*/
func (market *Market) Warmup(afterStep func() error) error {
	if !market.bootstrapped {
		return errnie.Err(
			errnie.Validation,
			"tests: market must be bootstrapped before warming",
			nil,
		)
	}

	return market.Transition(MarketStateBaseline, afterStep)
}

/*
Transition emits one validated semantic scenario, optionally isolating its
event to selected symbols while every other market continues idling.
*/
func (market *Market) Transition(
	state MarketState, afterStep func() error, symbols ...string,
) error {
	if !market.bootstrapped {
		return errnie.Err(
			errnie.Validation,
			"tests: market must be bootstrapped before transitioning",
			nil,
		)
	}

	if afterStep == nil {
		return errnie.Err(errnie.Validation, "tests: transition step callback required", nil)
	}

	steps, err := market.signal.Scenario(state, symbols...)

	if err != nil {
		return errnie.Err(errnie.Validation, "tests: invalid market transition", err)
	}

	for _, step := range steps {
		if err := market.Apply(step, afterStep); err != nil {
			return err
		}
	}

	market.signal.Complete(state)
	market.State = state
	return nil
}

/*
Apply advances one explicit economic step, validates all derived feeds, emits
them in causal order, and lets the production graph consume that observation.
*/
func (market *Market) Apply(
	step marketsignal.Step,
	afterStep func() error,
) (err error) {
	if !market.bootstrapped {
		return errnie.Err(
			errnie.Validation,
			"tests: market must be bootstrapped before applying events",
			nil,
		)
	}

	if err := market.ctx.Err(); err != nil {
		return errnie.Err(errnie.Internal, "tests: market closed", err)
	}

	if market.failed != nil {
		return errnie.Err(errnie.Internal, "tests: market failed", market.failed)
	}

	for _, transport := range []struct {
		name string
		conn *mockapi.MockConn
	}{
		{"public", market.Public},
		{"paper", market.Paper},
		{"level3", market.Level3},
	} {
		if err := transport.conn.Err(); err != nil {
			market.failed = err
			return errnie.Err(
				errnie.Validation,
				"tests: production "+transport.name+" rejected frame",
				err,
			)
		}
	}

	if afterStep == nil {
		return errnie.Err(errnie.Validation, "tests: step callback required", nil)
	}

	if err := market.signal.Apply(step); err != nil {
		return errnie.Err(errnie.Validation, "tests: apply market step", err)
	}

	defer func() {
		if err != nil {
			market.failed = err
		}
	}()

	payloads, err := market.read(
		market.ticker, market.trade, market.book, market.level3,
	)

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

	if len(payloads.level3) > 0 {
		if err := market.Level3.Publish("level3", payloads.level3); err != nil {
			return errnie.Err(errnie.IO, "tests: queue level3 frame", err)
		}
	}

	for _, frame := range []struct {
		channel string
		payload []byte
	}{
		{"trade", payloads.trade},
		{"book", payloads.book},
		{"ticker", payloads.ticker},
	} {
		if len(frame.payload) == 0 {
			continue
		}

		if err := market.Public.Publish(frame.channel, frame.payload); err != nil {
			return errnie.Err(errnie.IO, "tests: queue "+frame.channel+" frame", err)
		}
	}

	if err := market.Level3.Drain(); err != nil {
		return errnie.Err(errnie.IO, "tests: drain level3 frames", err)
	}

	if err := market.Level3.Err(); err != nil {
		return errnie.Err(errnie.Validation, "tests: production level3 rejected frame", err)
	}

	if err := market.Public.Drain(); err != nil {
		return errnie.Err(errnie.IO, "tests: drain public frames", err)
	}

	if err := market.Public.Err(); err != nil {
		return errnie.Err(errnie.Validation, "tests: production public rejected frame", err)
	}

	if err := market.Paper.MatchPaper(); err != nil {
		return errnie.Err(errnie.Internal, "tests: match resting paper orders", err)
	}

	if err := market.Paper.Drain(); err != nil {
		return errnie.Err(errnie.IO, "tests: drain private frames", err)
	}

	if err := market.Paper.Err(); err != nil {
		return errnie.Err(errnie.Validation, "tests: production private rejected frame", err)
	}

	if err := afterStep(); err != nil {
		return errnie.Err(errnie.Internal, "tests: market step failed", err)
	}

	if err := market.Paper.Drain(); err != nil {
		return errnie.Err(errnie.IO, "tests: drain order responses", err)
	}

	if err := market.Paper.Err(); err != nil {
		return errnie.Err(errnie.Validation, "tests: production order response rejected", err)
	}

	return nil
}

/*
configure assigns fixture streams to the Kraken subscription and REST routes.
*/
func (market *Market) configure() {
	err := market.Paper.EnablePaper(mockapi.PaperOptions{
		Quote: func(symbol string) (float64, float64, float64, float64, bool) {
			quote, exists := market.signal.Quote(symbol)
			return quote.Bid, quote.BidQty, quote.Ask, quote.AskQty, exists
		},
		Now: market.signal.Now,
		Balances: map[string]float64{
			"USD": defaultPaperQuoteBalance,
		},
		MakerFee: defaultPaperMakerFee,
		TakerFee: defaultPaperTakerFee,
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
