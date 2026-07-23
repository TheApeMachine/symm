package stack

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Booter owns construction and startup of the complete production trading graph.
The command only supplies its process context; every runtime dependency is
created here so tests can replace transports without rebuilding the system.
*/
type Booter struct {
	ctx     context.Context
	cancel  context.CancelFunc
	channel chan []byte
	stages  *system.Booter
}

/*
NewBooter creates the one production composition root.
*/
func NewBooter(ctx context.Context) *Booter {
	ctx, cancel := context.WithCancel(ctx)

	return &Booter{
		ctx:    ctx,
		cancel: cancel,
	}
}

/*
compose creates the shared boot primitives only after their runtime
configuration is final, keeping live and deterministic test construction on
the same composition path.
*/
func (booter *Booter) compose() {
	booter.channel = make(
		chan []byte,
		viper.GetInt("system.websocket.channel.buffer"),
	)

	booter.stages = system.NewBooter(booter.ctx, booter.channel)
}

/*
Start constructs the live transports, boots the complete graph, and serves it.
*/
func (booter *Booter) Start() error {
	booter.compose()
	simulator := websocket.NewLatencySimulator(booter.stages)
	public := websocket.New(
		booter.ctx, simulator, false, websocket.PublicWebSocketURL,
	)
	private := websocket.New(
		booter.ctx, simulator, true, websocket.PrivateWebSocketURL,
	)

	var paper websocket.Conn

	if viper.GetString("trading.model") == "paper" {
		paper = websocket.NewPaper(booter.ctx, simulator)
	}

	wired, err := booter.boot(
		public,
		private,
		paper,
		nil,
		nil,
		false,
		simulator,
		public,
		private,
	)

	if err != nil {
		return errnie.Error(err)
	}

	defer func() {
		errnie.Error(wired.Close())
	}()

	return wired.UIHub.Serve()
}

/*
Test boots the complete production graph against a fixture-driven market.
*/
func (booter *Booter) Test(market *tests.Market) (*Stack, error) {
	restore := booter.configureTest(len(market.Symbols))
	booter.compose()

	if err := market.Bootstrap(); err != nil {
		restore()
		return nil, errnie.Error(err)
	}

	wired, err := booter.boot(
		market.Public,
		market.Private,
		market.Paper,
		market.Level3,
		market.Symbols,
		true,
	)

	if err != nil {
		restore()
		return nil, err
	}

	wired.restore = restore

	return wired, nil
}

/*
configureTest applies deterministic runtime configuration for one simulated
stack and returns the exact restoration applied when that stack closes.
*/
func (booter *Booter) configureTest(symbolCount int) func() {
	auditDir, err := os.MkdirTemp("", "symm-audit-*")

	if err != nil {
		panic(err)
	}

	settings := map[string]any{
		"system.websocket.channel.buffer":            4096,
		"system.actor.buffer":                        64,
		"trading.model":                              "paper",
		"trading.allocation.max_fraction":            0.2,
		"trading.slots.normal":                       2,
		"trading.slots.reserved":                     2,
		"cognitive.in_memory":                        true,
		"cognitive.tick_budget":                      10 * time.Millisecond,
		"system.data_path":                           auditDir,
		"system.audit.rotate_on_boot":                false,
		"market.quote_currency":                      "USD",
		"market.subscribe_batch":                     symbolCount,
		"market.subscribe_pace":                      0,
		"market.l3_enabled":                          true,
		"market.l3_depth":                            10,
		"market.l3_rate_limit":                       200,
		"market.baseline_halflife":                   30 * time.Second,
		"market.forecast.rls.initial_variance":       1.0,
		"market.forecast.rls.forgetting_factor":      1.0,
		"market.forecast.rls.calibration_confidence": 0.95,
		"signals.feed_timeline_capacity":             4096,
		"signals.feed_track_capacity":                512,
		"market.manifold.integration_interval":       0,
	}
	previous := make(map[string]any, len(settings))

	for key, value := range settings {
		previous[key] = viper.Get(key)
		viper.Set(key, value)
	}

	return func() {
		for key, value := range previous {
			viper.Set(key, value)
		}

		_ = os.RemoveAll(auditDir)
	}
}

/*
Stack exposes the production graph after Booter has initialized every stage.
*/
type Stack struct {
	API        *websocket.API
	Booter     *system.Booter
	Channel    chan []byte
	Tree       *dmt.Tree
	Price      *broker.Price
	Instrument *broker.Instrument
	Balance    *broker.Balance
	Desk       *broker.Desk
	Analyzer   *logic.Analyzer
	Planner    *strategy.Planner
	Crypto     *trader.Crypto
	UIHub      *ui.Hub
	Recorder   *audit.Recorder
	Thesis     *types.Thesis
	Signals    []types.Signal
	restore    func()
}

/*
boot assembles and initializes the graph against the supplied Conn transports.
When cutOwned is true (fixture tests), signals only append measurements and
Observe runs analyzer → planner → desk so those stages do not race the Actor
cascade on the shared Thesis.
*/
func (booter *Booter) boot(
	public websocket.Conn,
	private websocket.Conn,
	paper websocket.Conn,
	level3 websocket.Conn,
	symbols []string,
	cutOwned bool,
	preflight ...types.StatusReporter,
) (*Stack, error) {
	api := websocket.NewAPI(booter.ctx, public, private, paper)

	if level3 != nil {
		api.InjectLevel3(publicActor(level3), level3, symbols)
	}

	tree := dmt.NewTree("")

	recorder, err := booter.openRecorder()

	if err != nil {
		return nil, errnie.Error(err)
	}

	price := broker.NewPrice(api)
	instrument := broker.NewInstrument(api, price, booter.channel)
	balance := broker.NewBalance(api, nil, booter.channel)
	desk := broker.NewDesk(booter.ctx, api, instrument, price, balance)
	hub := ui.NewHub(booter.ctx, price, balance, booter.channel)
	thesis := types.NewThesis(booter.channel)
	hawkesSignal := hawkes.NewSignal(booter.ctx, booter.channel)

	depthflowSignal, err := depthflow.NewSignal(
		booter.ctx,
		booter.channel,
		viper.GetInt("signals.feed_track_capacity"),
	)

	if err != nil {
		return nil, errors.Join(errnie.Error(err), recorder.Close())
	}

	signals := []types.Signal{
		pumpdump.NewSignal(
			booter.ctx,
			booter.channel,
			viper.GetInt("signals.feed_track_capacity"),
		),
		liquidity.NewSignal(booter.ctx, booter.channel),
		toxicity.NewSignal(booter.ctx, api, booter.channel),
		leadlag.NewSignal(booter.ctx, booter.channel),
		cvd.NewSignal(booter.ctx, booter.channel),
		correlation.NewSignal(booter.ctx, booter.channel),
		exhaust.NewSignal(booter.ctx, booter.channel),
		sentiment.NewSignal(booter.ctx, booter.channel),
		depthflowSignal,
		hawkesSignal,
	}
	analyzer, err := logic.NewAnalyzer(
		booter.ctx,
		booter.stages,
		api,
		hawkesSignal,
		tree,
		booter.channel,
		recorder,
	)

	if err != nil {
		return nil, errors.Join(errnie.Error(err), recorder.Close())
	}

	planner := strategy.NewPlanner(
		booter.ctx,
		booter.channel,
		api,
		desk,
		instrument,
		price,
		balance,
		analyzer,
		strategy.NewAllocator(booter.ctx, balance, instrument, price),
		recorder,
	)

	crypto, err := trader.NewCrypto(
		booter.ctx,
		hub,
		recorder,
		desk,
	)

	if err != nil {
		return nil, errors.Join(errnie.Error(err), recorder.Close())
	}

	wired := &Stack{
		API:        api,
		Booter:     booter.stages,
		Channel:    booter.channel,
		Tree:       tree,
		Price:      price,
		Instrument: instrument,
		Balance:    balance,
		Desk:       desk,
		Analyzer:   analyzer,
		Planner:    planner,
		Crypto:     crypto,
		UIHub:      hub,
		Recorder:   recorder,
		Thesis:     thesis,
		Signals:    signals,
	}

	if reporter, ok := paper.(types.StatusReporter); ok {
		preflight = append(preflight, reporter)
	}

	market := publicActor(public)

	if market == nil {
		return nil, errors.Join(
			errnie.Error(errnie.Err(
				errnie.Validation,
				"boot: public transport has no Actor",
				nil,
			)),
			wired.Close(),
		)
	}

	// Desk must be attached and Running before Balance/Instrument subscribe,
	// or the first private/public snapshots publish to zero subscribers.
	transports := system.NewStage(
		system.StagePreflight, append(preflight, api, price)...,
	)
	balances := system.NewStage(system.StagePreflight, balance)
	instruments := system.NewStage(system.StagePreflight, instrument)
	booter.stages.AddStages(transports, balances, instruments)

	if err := transports.Initialize(booter.channel); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	if err := desk.Initialize(market, accountActor(api)); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	if err := balances.Initialize(booter.channel); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	if err := instruments.Initialize(booter.channel); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	for _, signal := range signals {
		signal.Initialize(market, thesis)
	}

	if cutOwned {
		return wired, nil
	}

	if err := analyzer.Initialize(
		types.Topic{Name: "ticker", Actor: hawkesSignal.Actor},
		types.Topic{Name: "book", Actor: hawkesSignal.Actor},
		types.Topic{Name: "trade", Actor: hawkesSignal.Actor},
	); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	if err := planner.Initialize(analyzer); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	if err := crypto.Initialize(planner); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	return wired, nil
}

/*
Observe waits for signal Actors to finish measuring the latest drained tape,
then runs analyzer, planner, and desk apply on the shared Thesis.
Thesis time follows the newest measurement At from the fixture tape.
*/
func (stack *Stack) Observe() (*types.Thesis, error) {
	stack.settle()

	measurements := stack.Thesis.SnapshotMeasurements()
	tick := stack.Crypto.NextTick()
	at := observationTime(measurements)
	stack.Thesis.ResetTick(at, tick)
	stack.Thesis.InstallMeasurements(measurements)

	errnie.Error(audit.Phase(stack.Recorder, tick, "tick_begin", nil))

	errnie.Error(audit.Phase(stack.Recorder, tick, "measure_end", map[string]any{
		"measurements": len(stack.Thesis.Measurements),
	}))

	stack.Analyzer.Update(stack.Thesis)

	errnie.Error(audit.Phase(stack.Recorder, tick, "decide_begin", nil))
	stack.Planner.Decide(stack.Thesis)
	errnie.Error(audit.Phase(stack.Recorder, tick, "decide_end", map[string]any{
		"decisions": len(stack.Thesis.Decisions),
	}))

	stack.Crypto.Apply(stack.Thesis)
	errnie.Error(audit.Phase(stack.Recorder, tick, "tick_end", nil))

	return stack.Thesis, nil
}

/*
observationTime picks the newest measurement timestamp so Thesis.At tracks the
fixture tape rather than wall clock.
*/
func observationTime(measurements []*types.Measurement) time.Time {
	at := time.Time{}

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.At.After(at) {
			at = measurement.At
		}
	}

	if at.IsZero() {
		return time.Now().UTC()
	}

	return at
}

func (stack *Stack) settle() {
	deadline := time.Now().Add(500 * time.Millisecond)
	previous := -1
	stable := 0

	for time.Now().Before(deadline) {
		count := len(stack.Thesis.SnapshotMeasurements())

		if count == previous {
			stable++

			if stable >= 5 {
				return
			}
		} else {
			stable = 0
			previous = count
		}

		time.Sleep(15 * time.Millisecond)
	}
}

/*
publicActor reads the embedded Actor from known public transports.
*/
func publicActor(conn websocket.Conn) *types.Actor {
	switch transport := conn.(type) {
	case *websocket.Live:
		return transport.Actor
	case *mockapi.MockConn:
		return transport.Actor
	case *websocket.Paper:
		return transport.Actor
	default:
		return nil
	}
}

/*
accountActor reads the embedded Actor from the private or paper transport.
*/
func accountActor(api *websocket.API) *types.Actor {
	if api == nil {
		return nil
	}

	return publicActor(api.Account())
}

/*
Close releases every resource owned by the assembled production graph and
returns all lifecycle failures after configuration has been restored.
*/
func (stack *Stack) Close() (err error) {
	if stack.Crypto != nil {
		err = errors.Join(err, stack.Crypto.Close())
	}

	if stack.API != nil {
		stack.API.Close()
	}

	if stack.UIHub != nil {
		err = errors.Join(err, stack.UIHub.Close())
	}

	if stack.Tree != nil {
		err = errors.Join(err, stack.Tree.Close())
	}

	if stack.Recorder != nil {
		err = errors.Join(err, stack.Recorder.Close())
	}

	if stack.restore != nil {
		stack.restore()
		stack.restore = nil
	}

	return err
}
