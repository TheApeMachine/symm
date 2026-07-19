package tests

import (
	"context"
	_ "embed"
	"iter"
	"path/filepath"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

//go:embed fixtures/instrument/fixtures/snapshot.json
var instrumentSnapshotBytes []byte

/*
Session is a thin driver over mock Conns + stack.Boot — the same production
graph as cmd/root.go, with producers substituted for live sockets. Feed
opportunity/trap tapes via Emit/Play; advance with Crypto.Tick. Strategy-only
truths given seeded forecasts use CommitStrategy and must say so.
*/
type Session struct {
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	channel    chan []byte
	clock      time.Time
	interval   time.Duration
	tree       *dmt.Tree
	stack      *stack.Stack
	paperState string

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
SignalFactory builds the signal set once Instrument exists. Callers supply it so
the tests package never imports concrete signals (avoids import cycles).
*/
type SignalFactory func(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal

/*
SessionOptions configures which signals participate. Signals must be provided.
*/
type SessionOptions struct {
	Signals  SignalFactory
	Clock    time.Time
	Interval time.Duration
	Level3   bool
}

/*
NewSession wires mock Conns into stack.Boot — Conn swap only, production graph.
*/
func NewSession(
	ctx context.Context,
	t testing.TB,
	options SessionOptions,
) (*Session, error) {
	t.Helper()

	env := NewSessionEnv()
	t.Cleanup(env.Configure(t))

	paperState := filepath.Join(t.TempDir(), "paper-state.json")
	InstallPaperCLI(t, paperState)

	sessionCtx, cancel := context.WithCancel(ctx)
	channel := make(chan []byte, 256)
	mock := NewMockAPI()

	if err := configureDefaultFees(mock); err != nil {
		cancel()
		return nil, err
	}

	api, paper, err := mock.Wire(sessionCtx)

	if err != nil {
		cancel()
		return nil, err
	}

	if options.Signals == nil {
		cancel()
		return nil, errnie.Err(
			errnie.Validation,
			"tests: SessionOptions.Signals factory is required",
			nil,
		)
	}

	tree := dmt.NewTree(t.TempDir())

	t.Cleanup(func() {
		errnie.Error(tree.Close())
	})

	wired, err := stack.Boot(sessionCtx, api, stack.Options{
		Paper:   paper,
		Channel: channel,
		Tree:    tree,
		Signals: stack.SignalFactory(options.Signals),
		FeedInstrument: func() {
			mock.Emit("instrument", instrumentSnapshotBytes)
		},
	})

	if err != nil {
		cancel()
		return nil, errnie.Err(errnie.Internal, "tests: stack boot", err)
	}

	var books *SessionLevel3

	if options.Level3 {
		books = NewSessionLevel3(sessionCtx, api)
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
		ctx:        sessionCtx,
		cancel:     cancel,
		api:        api,
		channel:    channel,
		clock:      clock,
		interval:   interval,
		tree:       tree,
		stack:      wired,
		paperState: paperState,
		Mock:       mock,
		Paper:      paper,
		Crypto:     wired.Crypto,
		Planner:    wired.Planner,
		Desk:       wired.Desk,
		Price:      wired.Price,
		Balance:    wired.Balance,
		Level3:     books,
	}

	t.Cleanup(func() {
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

	if session.stack != nil {
		session.stack.Close()
		return
	}

	if session.Crypto != nil {
		errnie.Error(session.Crypto.Close())
	}

	if session.api != nil {
		session.api.Close()
	}
}

/*
CommitStrategy runs Plan then Trade on an already-cut thesis. Use only when the
scenario's known truth is strategy/wallet behavior given seeded forecasts — not
as a substitute for Crypto.Tick market proof.
*/
func (session *Session) CommitStrategy(thesis *types.Thesis) error {
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
			"tests: thesis required for CommitStrategy",
			nil,
		)
	}

	if err := session.Crypto.Plan(thesis); err != nil {
		return err
	}

	session.Crypto.Trade(thesis)

	return nil
}

/*
Emit delivers one public frame through the Conn emulator into registered
handlers. Level3 frames use the Session L3 write lease when enabled.
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
ticker frame arrives so the real Cut→Update→Plan→trade path warms across the
stream.
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

func configureDefaultFees(mock *MockAPI) error {
	fee := kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)}
	fees := map[string]kraken.TradeVolumeFee{
		"BTC/USD":   fee,
		"MATIC/USD": fee,
		"ETH/USD":   fee,
		"WEAK/USD":  fee,
	}

	return mock.SetTradeVolumeResponse(&kraken.TradeVolume{
		Result: kraken.TradeVolumeResult{Fees: fees},
	})
}
