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
	"github.com/theapemachine/symm/config"
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
	config  config.Config
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
func (booter *Booter) compose() error {
	cfg, err := config.Load()

	if err != nil {
		return err
	}

	booter.config = cfg
	booter.channel = make(chan []byte, cfg.System.ChannelBuffer)
	booter.stages = system.NewBooter(booter.ctx, booter.channel)

	return nil
}

/*
Start constructs the live transports, boots the complete graph, and serves it.
*/
func (booter *Booter) Start() error {
	if err := booter.compose(); err != nil {
		return errnie.Error(err)
	}

	simulator := websocket.NewLatencySimulator(booter.ctx, websocket.WallClock{}, 0)
	public := websocket.New(
		booter.ctx, simulator, false, websocket.PublicWebSocketURL,
	)
	private := websocket.New(
		booter.ctx, simulator, true, websocket.PrivateWebSocketURL,
	)

	var paper websocket.Conn

	if booter.config.Trading.Model == "paper" {
		paper = websocket.NewPaper(booter.ctx, simulator, booter.config)
	}

	wired, err := booter.boot(
		public,
		private,
		paper,
		nil,
		nil,
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

	if err := booter.compose(); err != nil {
		restore()
		return nil, errnie.Error(err)
	}

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
		"market.subscribe_pace":                      20 * time.Millisecond,
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
		"ui.addr":                                    "127.0.0.1:0",
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
closer is one reverse-construction teardown step owned by Stack.
*/
type closer struct {
	name string
	fn   func() error
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
	config     config.Config
	cancel     context.CancelFunc
	restore    func()
	closers    []closer
	actors     []*types.Actor
}

/*
boot assembles and initializes the graph against the supplied Conn transports.
Fixture tests and live start share the same Actor cascade: signals measure into
the shared Thesis; Hawkes feeds analyzer directly for manifold energy while the
other signals stream their updates independently onto that same thesis.
*/
func (booter *Booter) boot(
	public websocket.Conn,
	private websocket.Conn,
	paper websocket.Conn,
	level3 websocket.Conn,
	symbols []string,
	preflight ...types.StatusReporter,
) (*Stack, error) {
	api := websocket.NewAPI(booter.ctx, public, private, paper)

	if level3 != nil {
		api.InjectLevel3(level3.Root(), level3, symbols)
	}

	recorder, err := booter.openRecorder()

	if err != nil {
		return nil, errnie.Error(err)
	}

	tree, err := dmt.NewTree("")

	if err != nil {
		return nil, errors.Join(errnie.Error(err), recorder.Close())
	}

	price := broker.NewPrice(api)
	instrument := broker.NewInstrument(api, price, booter.channel, booter.config.Market)
	recovery, err := types.LoadRecovery(booter.config.System.DataPath)

	if err != nil {
		return nil, errors.Join(errnie.Error(err), recorder.Close())
	}

	seed := make([]types.Holding, 0)

	if recovery != nil {
		for _, holding := range recovery.Holdings {
			seed = append(seed, holding)
		}
	}

	balance := broker.NewBalance(api, seed, booter.channel, booter.config.Market)
	desk := broker.NewDesk(
		booter.ctx, api, instrument, price, balance, booter.config.Trading,
	)
	hub := ui.NewHub(booter.ctx, price, balance, booter.channel, booter.config.UI)
	thesis := types.NewThesis()
	hawkesSignal := hawkes.NewSignal(booter.ctx, booter.channel)

	trackCapacity := booter.config.Signals.FeedTrackCapacity

	depthflowSignal, err := depthflow.NewSignal(
		booter.ctx,
		booter.channel,
		trackCapacity,
	)

	if err != nil {
		return nil, errors.Join(errnie.Error(err), recorder.Close())
	}
	pumpdumpSignal := pumpdump.NewSignal(
		booter.ctx,
		booter.channel,
		trackCapacity,
	)
	liquiditySignal := liquidity.NewSignal(booter.ctx, booter.channel)
	toxicitySignal := toxicity.NewSignal(booter.ctx, api, booter.channel)
	leadlagSignal := leadlag.NewSignal(booter.ctx, booter.channel)
	cvdSignal := cvd.NewSignal(booter.ctx, booter.channel)
	correlationSignal := correlation.NewSignal(booter.ctx, booter.channel)
	exhaustSignal := exhaust.NewSignal(booter.ctx, booter.channel)
	sentimentSignal := sentiment.NewSignal(booter.ctx, booter.channel)

	signals := []types.Signal{
		pumpdumpSignal,
		liquiditySignal,
		toxicitySignal,
		leadlagSignal,
		cvdSignal,
		correlationSignal,
		exhaustSignal,
		sentimentSignal,
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
		strategy.NewAllocator(
			booter.ctx, balance, instrument, price,
		),
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
		config:     booter.config,
		cancel:     booter.cancel,
		actors: []*types.Actor{
			desk.Actor,
			analyzer.Actor,
			planner.Actor,
			crypto.Actor,
		},
	}

	wired.closers = []closer{
		{name: "crypto", fn: crypto.Close},
		{name: "planner", fn: planner.Close},
		{name: "analyzer", fn: analyzer.Close},
		{name: "api", fn: func() error { api.Close(); return nil }},
		{name: "hub", fn: hub.Close},
		{name: "tree", fn: tree.Close},
		{name: "recorder", fn: recorder.Close},
	}

	if reporter, ok := paper.(types.StatusReporter); ok {
		preflight = append(preflight, reporter)
	}

	market := public.Root()

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

	if err := transports.Initialize(booter.ctx, booter.channel); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	if err := desk.Initialize(market, api.Account().Root()); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	// Signals must Subscribe before Instrument market subscribe+snapshot, or the
	// first book frames publish to zero signal subscribers and exhaust/toxicity
	// baselines never see resting depth.
	for _, signal := range signals {
		signal.Initialize(market, thesis)
	}

	if err := balances.Initialize(booter.ctx, booter.channel); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	if err := instruments.Initialize(booter.ctx, booter.channel); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	// Set the shared thesis pointer so onSignal can snapshot measurements.
	analyzer.SetThesis(thesis)

	// Wire every signal to the Analyzer. Hawkes drives the manifold through
	// ticker/trade at depth one; all other signals publish SignalResult to
	// their "thesis" topic which the Analyzer's "thesis" handler processes
	// for category composition, cognition, and UI publish.
	signalTopics := []types.Topic{
		{Name: "ticker", Actor: hawkesSignal.Actor},
		{Name: "trade", Actor: hawkesSignal.Actor},
		{Name: "thesis", Actor: pumpdumpSignal.Actor},
		{Name: "thesis", Actor: liquiditySignal.Actor},
		{Name: "thesis", Actor: toxicitySignal.Actor},
		{Name: "thesis", Actor: leadlagSignal.Actor},
		{Name: "thesis", Actor: cvdSignal.Actor},
		{Name: "thesis", Actor: correlationSignal.Actor},
		{Name: "thesis", Actor: exhaustSignal.Actor},
		{Name: "thesis", Actor: sentimentSignal.Actor},
		{Name: "thesis", Actor: depthflowSignal.Actor},
	}

	if err := analyzer.Initialize(signalTopics...); err != nil {
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
Close cancels the root context, waits for every registered actor and signal,
then closes stateful dependencies in reverse construction order.
*/
func (stack *Stack) Close() (err error) {
	err = errors.Join(err, stack.saveRecovery())

	if stack.cancel != nil {
		stack.cancel()
		stack.cancel = nil
	}

	for _, signal := range stack.Signals {
		err = errors.Join(err, signal.Close())
	}

	for _, actor := range stack.actors {
		err = errors.Join(err, actor.Close())
	}

	for _, step := range stack.closers {
		if step.fn == nil {
			continue
		}

		err = errors.Join(err, step.fn())
	}

	if stack.restore != nil {
		stack.restore()
		stack.restore = nil
	}

	return err
}

/*
saveRecovery captures open holdings and writes recovery.json under data_path.
*/
func (stack *Stack) saveRecovery() error {
	if stack == nil || stack.Balance == nil || stack.config.System.DataPath == "" {
		return nil
	}

	holdings := map[string]types.Holding{}

	for holding := range stack.Balance.Lots() {
		holdings[holding.Symbol] = holding
	}

	recovery, err := types.CaptureRecovery(stack.Thesis.Tick, holdings, nil, nil)

	if err != nil {
		return err
	}

	return types.SaveRecovery(stack.config.System.DataPath, recovery)
}
