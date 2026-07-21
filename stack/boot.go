package stack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytedance/sonic"
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
	"github.com/theapemachine/symm/signal/fluid"
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
	test    bool
}

/*
NewBooter creates the one production composition root.
*/
func NewBooter(ctx context.Context) *Booter {
	ctx, cancel := context.WithCancel(ctx)
	channel := make(chan []byte, viper.GetInt("system.websocket.channel.buffer"))

	return &Booter{
		ctx:     ctx,
		cancel:  cancel,
		channel: channel,
		stages:  system.NewBooter(ctx, channel),
	}
}

/*
Start constructs the live transports, boots the complete graph, and serves it.
*/
func (booter *Booter) Start() error {
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

	defer wired.Close()

	if err := wired.Crypto.Run(); err != nil {
		return errnie.Error(err)
	}

	return wired.UIHub.Serve()
}

/*
Test boots the complete production graph against a fixture-driven market.
*/
func (booter *Booter) Test(market *tests.Market) (*Stack, error) {
	booter.test = true
	viper.Set("trading.model", "paper")
	viper.Set("cognitive.in_memory", true)
	viper.Set("system.data_path", "")
	viper.Set("system.audit.rotate_on_boot", false)
	viper.Set("market.quote_currency", "USD")
	viper.Set("market.subscribe_batch", len(market.Symbols))
	viper.Set("market.subscribe_pace", 0)
	viper.Set("market.l3_enabled", true)
	viper.Set("market.l3_depth", 10)
	viper.Set("market.l3_rate_limit", 200)
	viper.Set("market.baseline_halflife", 30*time.Second)
	viper.Set("market.forecast.rls.initial_variance", 1.0)
	viper.Set("market.forecast.rls.forgetting_factor", 1.0)
	viper.Set("market.forecast.rls.calibration_confidence", 0.95)
	viper.Set("signals.feed_timeline_capacity", 4096)
	viper.Set("signals.feed_track_capacity", 512)
	viper.Set("signals.fluid.grid_half_width", 0)
	viper.Set("signals.fluid.integration_interval", 0)
	viper.Set("signals.fluid.idle_threshold", 5*time.Second)
	viper.Set("signals.fluid.max_integration_steps", 50)
	viper.Set("trading.slots.normal", 2)
	viper.Set("trading.slots.reserved", 2)

	return booter.boot(
		market.Public,
		market.Private,
		market.Paper,
		market.Level3,
		market.Symbols,
	)
}

/*
Stack exposes the production graph after Booter has initialized every stage.
*/
type Stack struct {
	API        *websocket.API
	Booter     *system.Booter
	Channel    chan []byte
	Tree       *dmt.Tree
	Thesis     *types.Thesis
	Price      *broker.Price
	Instrument *broker.Instrument
	Balance    *broker.Balance
	Desk       *broker.Desk
	Analyzer   *logic.Analyzer
	Planner    *strategy.Planner
	Crypto     *trader.Crypto
	UIHub      *ui.Hub
	Recorder   *audit.Recorder
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
	tree, dataPath, recorder, err := booter.storage()

	if err != nil {
		return nil, err
	}

	api := websocket.NewAPI(booter.ctx, public, private, paper)

	if level3 != nil {
		api.InjectLevel3(level3, symbols)
	}
	price := broker.NewPrice(api)
	instrument := broker.NewInstrument(api, price, booter.channel)
	balance := broker.NewBalance(api, nil, booter.channel)
	desk := broker.NewDesk(api, instrument, price, balance)
	thesis := booter.thesis(dataPath)
	hub := ui.NewHub(booter.ctx, price, balance, booter.channel)
	hawkesSignal := hawkes.NewSignal(booter.ctx, api, booter.channel)
	signals := []types.Signal{
		pumpdump.NewSignal(
			booter.ctx,
			api,
			booter.channel,
			viper.GetInt("signals.feed_track_capacity"),
		),
		liquidity.NewSignal(booter.ctx, api, booter.channel),
		toxicity.NewSignal(booter.ctx, api, booter.channel),
		leadlag.NewSignal(booter.ctx, api, booter.channel),
		cvd.NewSignal(booter.ctx, api, booter.channel),
		correlation.NewSignal(booter.ctx, api, booter.channel),
		exhaust.NewSignal(booter.ctx, api, instrument, booter.channel),
		sentiment.NewSignal(booter.ctx, api, booter.channel),
		depthflow.NewSignal(booter.ctx, api, instrument, booter.channel),
		fluid.NewSignal(booter.ctx, api, instrument, booter.channel),
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
		return nil, errnie.Error(err)
	}

	hub.BindFocus(analyzer.Focus)
	planner := strategy.NewPlanner(
		booter.ctx,
		booter.channel,
		api,
		desk,
		instrument,
		price,
		balance,
		signals,
		analyzer,
		strategy.NewAllocator(booter.ctx, balance, instrument, price),
		recorder,
	)
	crypto, err := trader.NewCrypto(
		booter.ctx,
		booter.stages,
		api,
		price,
		balance,
		desk,
		instrument,
		analyzer,
		planner,
		tree,
		thesis,
		hub,
		recorder,
	)

	if err != nil {
		return nil, errnie.Error(err)
	}

	wired := &Stack{
		API:        api,
		Booter:     booter.stages,
		Channel:    booter.channel,
		Tree:       tree,
		Thesis:     thesis,
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

	preflight = append(preflight, api, instrument, balance, price, desk)
	booter.stages.AddStages(
		system.NewStage(system.StagePreflight, preflight...),
		system.NewStage(system.StageWarmup, crypto),
		system.NewStage(system.StageReady, analyzer, planner),
	)

	if err := booter.stages.Start(); err != nil {
		wired.Close()
		return nil, errnie.Error(err)
	}

	return wired, nil
}

/*
storage opens the configured cognitive tree, data directory, and audit stream.
*/
func (booter *Booter) storage() (*dmt.Tree, string, *audit.Recorder, error) {
	if booter.test {
		recorder, err := audit.NewRecorder(os.DevNull)

		return dmt.NewTree(""), "", recorder, err
	}

	persistDir, err := configuredPath("cognitive.persist_dir")

	if err != nil {
		return nil, "", nil, err
	}

	inMemory := viper.GetBool("cognitive.in_memory")

	if !inMemory && persistDir == "" {
		return nil, "", nil, errnie.Err(
			errnie.Validation,
			"cognitive.persist_dir is required unless cognitive.in_memory is set",
			nil,
		)
	}

	if !inMemory && viper.GetBool("cognitive.reset_on_boot") {
		if _, statErr := os.Stat(persistDir); statErr == nil {
			if err := os.Rename(persistDir, audit.RotatedPath(persistDir)); err != nil {
				return nil, "", nil, errnie.Err(
					errnie.IO, "failed to rotate cognitive store", err,
				)
			}
		} else if !os.IsNotExist(statErr) {
			return nil, "", nil, errnie.Err(
				errnie.IO, "failed to inspect cognitive store", statErr,
			)
		}
	}

	dataPath, err := configuredPath("system.data_path")

	if err != nil {
		return nil, "", nil, err
	}

	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		return nil, "", nil, errnie.Err(
			errnie.IO, "failed to create data directory", err,
		)
	}

	auditDir := persistDir

	if inMemory || auditDir == "" {
		auditDir = dataPath
	}

	auditPath := filepath.Join(auditDir, "runtime-audit.jsonl")

	if viper.GetBool("system.audit.rotate_on_boot") {
		if err := audit.Rotate(auditPath); err != nil {
			return nil, "", nil, errnie.Err(
				errnie.IO, "failed to rotate runtime audit", err,
			)
		}
	}

	recorder, err := audit.NewRecorder(auditPath)

	if err != nil {
		return nil, "", nil, errnie.Err(
			errnie.IO, "failed to create runtime audit recorder", err,
		)
	}

	treeDir := persistDir

	if inMemory {
		treeDir = ""
	}

	return dmt.NewTree(treeDir), dataPath, recorder, nil
}

/*
thesis restores the optional prior thesis and removes stale inventory lots.
*/
func (booter *Booter) thesis(dataPath string) *types.Thesis {
	thesis := types.NewThesis(booter.channel, nil)
	encoded, err := os.ReadFile(filepath.Join(dataPath, "thesis.json"))

	if err == nil {
		restored := types.NewThesis(booter.channel, nil)

		if err := sonic.Unmarshal(encoded, restored); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"failed to unmarshal optional thesis",
				err,
			))
		} else {
			thesis = restored
		}
	} else if !os.IsNotExist(err) {
		errnie.Error(errnie.Err(
			errnie.IO, "failed to read optional thesis", err,
		))
	}

	thesis.Holdings.Range(func(key, value any) bool {
		thesis.Holdings.Delete(key)
		return true
	})

	return thesis
}

/*
Close releases every resource owned by the assembled production graph.
*/
func (stack *Stack) Close() {
	if stack.Crypto != nil {
		errnie.Error(stack.Crypto.Close())
	}

	if stack.API != nil {
		stack.API.Close()
	}

	if stack.UIHub != nil {
		errnie.Error(stack.UIHub.Close())
	}

	if stack.Tree != nil {
		errnie.Error(stack.Tree.Close())
	}

	if stack.Recorder != nil {
		errnie.Error(stack.Recorder.Close())
	}
}

/*
configuredPath resolves one absolute or home-relative configured directory.
*/
func configuredPath(key string) (string, error) {
	path := strings.TrimSpace(viper.GetString(key))

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()

		if err != nil {
			return "", errnie.Err(errnie.IO, "failed to resolve "+key, err)
		}

		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}

	if path != "" && !filepath.IsAbs(path) {
		return "", errnie.Err(
			errnie.Validation,
			key+" must be absolute or home-relative",
			nil,
		)
	}

	return path, nil
}
