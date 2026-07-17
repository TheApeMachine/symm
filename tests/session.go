package tests

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
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
	mock     *MockAPI
	api      *websocket.API
	paper    *websocket.Paper
	crypto   *trader.Crypto
	planner  *strategy.Planner
	desk     *broker.Desk
	channel  chan []byte
	clock    time.Time
	interval time.Duration
	tree     *dmt.Tree
	level3   *websocket.Live
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
func NewSession(testingTB testing.TB, options SessionOptions) (*Session, error) {
	testingTB.Helper()

	previousModel := viper.Get("trading.model")
	previousData := viper.Get("system.data_path")
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	previousSlots := viper.Get("trading.slots.normal")
	previousReserved := viper.Get("trading.slots.reserved")
	previousQuote := viper.Get("market.quote_currency")
	previousBatch := viper.Get("market.subscribe_batch")
	previousPace := viper.Get("market.subscribe_pace")

	testingTB.Cleanup(func() {
		viper.Set("trading.model", previousModel)
		viper.Set("system.data_path", previousData)
		viper.Set("signals.feed_timeline_capacity", previousTimeline)
		viper.Set("signals.feed_track_capacity", previousTrack)
		viper.Set("trading.slots.normal", previousSlots)
		viper.Set("trading.slots.reserved", previousReserved)
		viper.Set("market.quote_currency", previousQuote)
		viper.Set("market.subscribe_batch", previousBatch)
		viper.Set("market.subscribe_pace", previousPace)
	})

	dataPath := testingTB.TempDir()
	viper.Set("trading.model", "paper")
	viper.Set("system.data_path", dataPath)
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)
	viper.Set("trading.slots.normal", 2)
	viper.Set("trading.slots.reserved", 2)
	viper.Set("market.quote_currency", "USD")
	viper.Set("market.subscribe_batch", 200)
	viper.Set("market.subscribe_pace", "20ms")

	ctx, cancel := context.WithCancel(context.Background())
	channel := make(chan []byte, 256)
	mock := NewMockAPI()
	api, paper, err := mock.Wire(ctx)

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
	// Register the handler without SubscribeInstruments — the mock Conn has no
	// live socket write path. Conditions prefix an instrument snapshot so Pair()
	// resolves before book frames arrive.
	api.On("instrument", instrument.On)

	balance := broker.NewBalance(api, nil, channel)
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

	var level3 *websocket.Live

	if options.Level3 {
		level3 = websocket.New(ctx, nil, true, websocket.Level3WebSocketURL)
		api.AttachLevel3(level3)
	}

	signals := options.Signals(ctx, api, instrument, channel)

	if len(signals) == 0 {
		cancel()
		return nil, errnie.Err(
			errnie.Validation,
			"tests: SessionOptions.Signals returned no signals",
			nil,
		)
	}

	planner := strategy.NewPlanner(ctx, channel, signals, nil)
	tree := dmt.NewTree(testingTB.TempDir())
	testingTB.Cleanup(func() {
		errnie.Error(tree.Close())
	})

	thesis := types.NewThesis(channel, nil)
	booter := system.NewBooter(ctx, channel)
	crypto, err := trader.NewCrypto(
		ctx,
		booter,
		api,
		price,
		balance,
		desk,
		instrument,
		nil,
		planner,
		tree,
		thesis,
		nil,
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
		ctx:      ctx,
		cancel:   cancel,
		mock:     mock,
		api:      api,
		paper:    paper,
		crypto:   crypto,
		planner:  planner,
		desk:     desk,
		channel:  channel,
		clock:    clock,
		interval: interval,
		tree:     tree,
		level3:   level3,
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

	if session.crypto != nil {
		errnie.Error(session.crypto.Close())
	}

	if session.api != nil {
		session.api.Close()
	}
}

/*
Mock returns the Conn emulator used to inject public market frames.
*/
func (session *Session) Mock() *MockAPI {
	return session.mock
}

/*
Crypto exposes the runtime under test for direct Tick assertions.
*/
func (session *Session) Crypto() *trader.Crypto {
	return session.crypto
}

/*
Desk exposes the paper desk for open-position assertions.
*/
func (session *Session) Desk() *broker.Desk {
	return session.desk
}

/*
Planner exposes the strategy planner wired into Crypto.Tick.
*/
func (session *Session) Planner() *strategy.Planner {
	return session.planner
}

/*
Paper exposes the paper simulator attached to the websocket API.
*/
func (session *Session) Paper() *websocket.Paper {
	return session.paper
}

/*
Clock returns the next virtual cut time.
*/
func (session *Session) Clock() time.Time {
	return session.clock
}

/*
Emit delivers one public frame through the Conn emulator into registered
handlers (Market, Price, …). Level3 frames are applied under the Session L3
write lease when Level3 was enabled.
*/
func (session *Session) Emit(frame Frame) {
	if frame.Channel == "level3" && session.level3 != nil {
		session.level3.ApplyLevel3(frame.Payload)
		return
	}

	session.mock.Emit(frame)
}

/*
Level3Enabled reports whether PeekBook-backed books are attached.
*/
func (session *Session) Level3Enabled() bool {
	return session.level3 != nil
}

/*
SeedTouch installs a two-sided L3 quote for toxicity Session tests.
*/
func (session *Session) SeedTouch(
	symbol string,
	bid float64,
	ask float64,
	quantity float64,
) {
	if session == nil || session.level3 == nil {
		return
	}

	session.level3.SeedTouch(symbol, bid, ask, quantity, session.clock)
}

/*
Advance emits frames, runs one Crypto.Tick at the virtual clock, then advances
the clock by the session interval.
*/
func (session *Session) Advance(frames ...Frame) (*types.Thesis, error) {
	for _, frame := range frames {
		session.Emit(frame)
	}

	thesis, err := session.crypto.Tick(session.clock)
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

		thesis, err := session.tickNow()

		if err != nil {
			return theses, err
		}

		pending = false

		if thesis != nil {
			theses = append(theses, thesis)
		}
	}

	// Flush only when no ticker-driven tick completed, so trailing book/trade
	// frames alone cannot replace a measured thesis with an empty quote cut.
	if !pending || len(theses) > 0 {
		return theses, nil
	}

	thesis, err := session.tickNow()

	if err != nil {
		return theses, err
	}

	if thesis != nil {
		theses = append(theses, thesis)
	}

	return theses, nil
}

func (session *Session) tickNow() (*types.Thesis, error) {
	thesis, err := session.crypto.Tick(session.clock)
	session.clock = session.clock.Add(session.interval)

	return thesis, err
}
