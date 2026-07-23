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

	wired.Crypto.Run()
	wired.Desk.Run()
	wired.Analyzer.Run()
	wired.Planner.Run()
	wired.API.Run()

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
	restore    func()
}

/*
boot assembles and initializes the graph against the supplied Conn transports.
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
		api.InjectLevel3(level3, symbols)
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
	}

	if reporter, ok := paper.(types.StatusReporter); ok {
		preflight = append(preflight, reporter)
	}

	preflight = append(preflight, api, price, instrument, balance, desk)
	booter.stages.AddStages(
		system.NewStage(system.StagePreflight, preflight...),
		system.NewStage(system.StageWarmup, crypto),
		system.NewStage(system.StageReady, analyzer, planner),
	)

	if err := booter.stages.Start(); err != nil {
		return nil, errors.Join(errnie.Error(err), wired.Close())
	}

	wired.startActors()

	return wired, nil
}

/*
startActors drains every Actor inbox so market frames, marks, and strategy
updates cannot block on an unread delivery during Test or live Start.
Crypto.Actor runs without the cut loop; live Start still calls Crypto.Run.
*/
func (stack *Stack) startActors() {
	stack.API.Run()
	stack.Desk.Run()
	stack.Analyzer.Run()
	stack.Planner.Run()
	stack.Crypto.Actor.Run()
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
