package tests

import (
	"context"
	"iter"
	"path/filepath"
	"testing"
	"time"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

/*
Session is a controllable paper-market harness. It injects MockConn transports
through websocket.NewAPI, feeds fixture frames via Emit, and advances the real
Crypto.Tick path so signals measure through Market.Cut rather than hand-built
frames. Add a condition by composing NewMarket feeds with scenario shapers
(Spike, TradeAggression, BookDecay, Cohort, …) and calling Play. Market tests
must falsify calm versus stressed PeakMetric/PeakSourceMetric outcomes.
*/
type Session struct {
	ctx      context.Context
	cancel   context.CancelFunc
	api      *websocket.API
	channel  chan []byte
	clock    time.Time
	interval time.Duration
	tree     *dmt.Tree

	Mock    *MockAPI
	Paper   *websocket.Paper
	Crypto  *trader.Crypto
	Planner *strategy.Planner
	Desk    *broker.Desk
	Price   *broker.Price
	Balance *broker.Balance
	Level3  *SessionLevel3
}

/*
SignalFactory builds the signal set once the Session has a live API, instrument,
and UI channel. Callers supply it so the tests package never imports concrete
signals (and so signal package tests can use Session without an import cycle).
*/
type SignalFactory func(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal

/*
SessionOptions configures which signals participate in a Session. Signals must
be provided — the harness stays signal-agnostic so packages under signal/ can
drive Session without cycling imports through tests.
*/
type SessionOptions struct {
	Signals  SignalFactory
	Clock    time.Time
	Interval time.Duration
	Level3   bool
}

/*
NewSession builds a paper stack against temporary data and cognitive paths.
*/
func NewSession(
	ctx context.Context,
	testingTB testing.TB,
	options SessionOptions,
) (*Session, error) {
	testingTB.Helper()

	env := NewSessionEnv()
	testingTB.Cleanup(env.Configure(testingTB))

	// Isolate paper CLI before Balance.Initialize so SubscribeBalance cannot
	// adopt the operator's live paper wallet into OpenPositions.
	InstallPaperCLI(testingTB, filepath.Join(testingTB.TempDir(), "paper-state.json"))

	sessionCtx, cancel := context.WithCancel(ctx)
	channel := make(chan []byte, 256)
	mock := NewMockAPI()
	api, paper, err := mock.Wire(sessionCtx)

	if err != nil {
		cancel()
		return nil, err
	}

	price := broker.NewPrice(api)

	if err := price.Initialize(); err != nil {
		cancel()
		return nil, errnie.Err(errnie.Internal, "tests: price initialize", err)
	}

	instrument := broker.NewInstrument(api, price, channel)
	api.On("instrument", instrument.On)

	balance := broker.NewBalance(api, nil, channel)

	if err := balance.Initialize(); err != nil {
		cancel()
		return nil, errnie.Err(errnie.Internal, "tests: balance initialize", err)
	}

	desk := broker.NewDesk(api, instrument, price, balance)

	if err := desk.Initialize(); err != nil {
		cancel()
		return nil, errnie.Err(errnie.Internal, "tests: desk initialize", err)
	}

	if options.Signals == nil {
		cancel()
		return nil, errnie.Err(
			errnie.Validation,
			"tests: SessionOptions.Signals factory is required",
			nil,
		)
	}

	var books *SessionLevel3

	if options.Level3 {
		books = NewSessionLevel3(sessionCtx, api)
	}

	signals := options.Signals(sessionCtx, api, instrument, channel)

	if len(signals) == 0 {
		cancel()
		return nil, errnie.Err(
			errnie.Validation,
			"tests: SessionOptions.Signals returned no signals",
			nil,
		)
	}

	analyzer, err := logic.NewAnalyzer(sessionCtx, nil, api, nil, nil, nil)

	if err != nil {
		cancel()
		return nil, errnie.Err(errnie.Internal, "tests: analyzer initialize", err)
	}

	planner := strategy.NewPlanner(
		sessionCtx,
		channel,
		api,
		desk,
		instrument,
		price,
		balance,
		signals,
		analyzer,
		strategy.NewAllocator(
			sessionCtx, balance, instrument, price,
		),
		nil,
	)

	tree := dmt.NewTree(testingTB.TempDir())

	testingTB.Cleanup(func() {
		errnie.Error(tree.Close())
	})

	crypto, err := env.Crypto(
		sessionCtx,
		api,
		price,
		balance,
		desk,
		instrument,
		planner,
		tree,
		channel,
	)

	if err != nil {
		cancel()
		return nil, err
	}

	clock := options.Clock

	if clock.IsZero() {
		clock = time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	}

	interval := options.Interval

	if interval <= 0 {
		interval = 5 * time.Second
	}

	session := &Session{
		ctx:      sessionCtx,
		cancel:   cancel,
		api:      api,
		channel:  channel,
		clock:    clock,
		interval: interval,
		tree:     tree,
		Mock:     mock,
		Paper:    paper,
		Crypto:   crypto,
		Planner:  planner,
		Desk:     desk,
		Price:    price,
		Balance:  balance,
		Level3:   books,
	}

	testingTB.Cleanup(func() {
		session.Close()
	})

	return session, nil
}

/*
Close releases session resources.
*/
func (session *Session) Close() {
	if session == nil {
		return
	}

	session.cancel()

	if session.Crypto != nil {
		errnie.Error(session.Crypto.Close())
	}

	if session.api != nil {
		session.api.Close()
	}
}

/*
RunDecide applies the same Plan path Crypto.Tick uses after a market Play, so
reserved/rotate assertions can run on a thesis seeded from the emulator cut
without requiring the GPU analyzer to invent forecasts.
*/
func (session *Session) RunDecide(thesis *types.Thesis) error {
	if session == nil || session.Crypto == nil {
		return errnie.Err(
			errnie.NotFound,
			"tests: session crypto unavailable",
			nil,
		)
	}

	if thesis == nil {
		return errnie.Err(
			errnie.Validation,
			"tests: thesis required for decide",
			nil,
		)
	}

	session.Crypto.Plan(thesis)

	return nil
}

/*
RunTrade runs Plan then Trade — the same submission half of Crypto.Tick after a
cut — so Session tests can prove enter/exit fills without inventing a Cut frame.
*/
func (session *Session) RunTrade(thesis *types.Thesis) error {
	if err := session.RunDecide(thesis); err != nil {
		return err
	}

	session.Crypto.Trade(thesis)

	return nil
}

/*
Emit delivers one public frame through the Conn emulator into registered
handlers (Market, Price, …). Level3 frames are applied under the Session L3
write lease when Level3 was enabled.
*/
func (session *Session) Emit(frame Frame) {
	if frame.Channel == "level3" && session.Level3 != nil {
		errnie.Error(session.Level3.Apply(frame.Payload))
		return
	}

	session.Mock.Emit(frame.Channel, frame.Payload)
}

/*
Advance emits frames, runs one Crypto.Tick at the virtual clock, then advances
the clock by the session interval.
*/
func (session *Session) Advance(frames ...Frame) (*types.Thesis, error) {
	for _, frame := range frames {
		session.Emit(frame)
	}

	thesis, err := session.Crypto.Tick(session.clock)
	session.clock = session.clock.Add(session.interval)

	return thesis, err
}

/*
Play emits every frame from a market timeline and runs Crypto.Tick when a
ticker frame arrives so quote signals warm across the stream. A final Tick
runs only when the stream had pending non-ticker frames and never completed a
ticker-driven tick, so trailing books cannot wipe a measured thesis.
*/
func (session *Session) Play(frames iter.Seq[Frame]) ([]*types.Thesis, error) {
	theses := make([]*types.Thesis, 0)
	pending := false

	for frame := range frames {
		session.Emit(frame)
		pending = true

		if frame.Channel != "ticker" {
			continue
		}

		thesis, err := session.Crypto.Tick(session.clock)
		session.clock = session.clock.Add(session.interval)

		if err != nil {
			return theses, err
		}

		pending = false

		if thesis != nil {
			theses = append(theses, thesis.CutSnapshot())
		}
	}

	if !pending || len(theses) > 0 {
		return theses, nil
	}

	thesis, err := session.Crypto.Tick(session.clock)
	session.clock = session.clock.Add(session.interval)

	if err != nil {
		return theses, err
	}

	if thesis != nil {
		theses = append(theses, thesis.CutSnapshot())
	}

	return theses, nil
}
