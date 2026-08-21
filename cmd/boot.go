package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/regulator"
	signalcorrelation "github.com/theapemachine/symm/signal/correlation"
	signalcvd "github.com/theapemachine/symm/signal/cvd"
	signaldepthflow "github.com/theapemachine/symm/signal/depthflow"
	signalexhaust "github.com/theapemachine/symm/signal/exhaust"
	signalhawkes "github.com/theapemachine/symm/signal/hawkes"
	signalleadlag "github.com/theapemachine/symm/signal/leadlag"
	signalliquidity "github.com/theapemachine/symm/signal/liquidity"
	signalpumpdump "github.com/theapemachine/symm/signal/pumpdump"
	signalsentiment "github.com/theapemachine/symm/signal/sentiment"
	signaltoxicity "github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

/*
System is the assembled symm system. Run starts each registered long-lived
component after all queue connections have been wired.
*/
type System struct {
	ctx       context.Context
	cancel    context.CancelFunc
	err       error
	Hub       *ui.Hub
	Desk      *broker.Desk
	Planner   *strategy.Planner
	Analyzer  *logic.Analyzer
	Crypto    *trader.Crypto
	Regulator *regulator.Solver
	Signals   []types.Signal
	Thesis    *types.Thesis
	Systems   []Runnable
	closers   []func() error
	resources []func() error
	runDone   chan struct{}
	runMu     sync.Mutex
	running   bool
}

func (stack *System) Name() string { return "system" }

func (stack *System) Error() error {
	stack.runMu.Lock()
	defer stack.runMu.Unlock()

	return stack.err
}

func (stack *System) fail(err error) {
	if errors.Is(err, context.Canceled) && stack.ctx.Err() != nil {
		return
	}

	stack.runMu.Lock()

	if stack.err == nil {
		stack.err = err
	}

	stack.runMu.Unlock()
	stack.cancel()
}

func (stack *System) Holding(symbol string) int {
	if stack == nil || stack.Desk == nil {
		return 0
	}

	return stack.Desk.Holding(symbol)
}

func (stack *System) Close() error {
	if stack == nil {
		return nil
	}

	if stack.cancel != nil {
		stack.cancel()
	}

	for _, v := range slices.Backward(stack.closers) {
		if err := v(); err != nil {
			return err
		}
	}

	stack.runMu.Lock()
	running := stack.running
	runDone := stack.runDone
	stack.runMu.Unlock()

	if running && runDone != nil {
		<-runDone
	}

	for _, closeResource := range slices.Backward(stack.resources) {
		if err := closeResource(); err != nil {
			return err
		}
	}

	return nil
}

func (stack *System) Run() error {
	stack.runMu.Lock()

	if stack.runDone == nil {
		stack.runDone = make(chan struct{})
	}

	stack.running = true
	runDone := stack.runDone
	stack.runMu.Unlock()
	defer close(runDone)

	group, _ := errgroup.WithContext(stack.ctx)

	for _, system := range stack.Systems {
		group.Go(func() error {
			errnie.Info("starting system: " + system.Name())

			if err := system.Run(); err != nil {
				if errors.Is(err, context.Canceled) && stack.ctx.Err() != nil {
					return nil
				}

				stack.cancel()

				return errnie.Error(errnie.Err(
					errnie.Internal,
					"system failed: "+system.Name(),
					err,
				))
			}

			return nil
		})
	}

	err := group.Wait()

	if err != nil {
		stack.fail(err)
	}

	return stack.Error()
}

/*
Runnable is a component registered in a System.
*/
type Runnable interface {
	Name() string
	Run() error
	Error() error
}

/*
Boot assembles the full symm stack around the injected Thesis and transport
connections. Passing nil for public or private opens real Kraken websocket
connections; tests inject in-memory fixture connections instead.
*/
func Boot(
	ctx context.Context,
	thesis *types.Thesis,
	public websocket.Conn,
	private websocket.Conn,
	uiChannel *transport.MapReduce[*types.UIFrame],
) *System {
	return BootWithHub(ctx, thesis, public, private, uiChannel, nil)
}

func BootWithHub(
	ctx context.Context,
	thesis *types.Thesis,
	public websocket.Conn,
	private websocket.Conn,
	uiChannel *transport.MapReduce[*types.UIFrame],
	hub *ui.Hub,
	recorders ...*audit.Recorder,
) *System {
	viper.SetDefault("system.actor.buffer", 1024)
	var recorder *audit.Recorder

	if len(recorders) > 0 {
		recorder = recorders[0]
	}

	systemCtx, cancel := context.WithCancel(ctx)

	if live, ok := public.(*websocket.Live); ok {
		live.SetThesis(thesis)
	}

	if live, ok := private.(*websocket.Live); ok {
		live.SetThesis(thesis)
	}

	manifoldChannel := transport.NewMapReduce[types.FluidFrame](nil, nil, nil)

	tree, err := dmt.NewTree("")

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "boot: create logic tree", err))
		cancel()
		return nil
	}

	regulatorSolver, err := regulator.NewSolver(systemCtx, uiChannel)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "boot: create regulator", err))
		cancel()
		return nil
	}

	api := websocket.NewAPI(systemCtx, public, private)
	signals := []types.Signal{
		signalcorrelation.NewSignal(systemCtx, thesis),
		signalcvd.NewSignal(systemCtx, thesis),
		signaldepthflow.NewSignal(systemCtx, thesis),
		signalexhaust.NewSignal(systemCtx, thesis),
		signalhawkes.NewSignal(systemCtx, thesis),
		signalleadlag.NewSignal(systemCtx, thesis),
		signalliquidity.NewSignal(systemCtx, thesis),
		signalpumpdump.NewSignal(systemCtx, thesis, api),
		signalsentiment.NewSignal(systemCtx, thesis),
		signaltoxicity.NewSignal(systemCtx, thesis),
	}

	price := broker.NewPrice(api)
	instrument := broker.NewInstrument(api, price, uiChannel)
	balance := broker.NewBalance(api, uiChannel)
	positionStore, err := broker.NewPositionStore(
		filepath.Join(utils.ResolveDataPath(), "symm.sqlite"),
	)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "boot: open position store", err))
		cancel()
		return nil
	}

	desk := broker.NewDesk(
		systemCtx,
		api,
		instrument,
		price,
		balance,
		thesis,
		regulatorSolver,
		nil,
		positionStore,
		uiChannel,
	)
	analyzer := logic.NewAnalyzer(
		systemCtx,
		price,
		api,
		tree,
		uiChannel,
		manifoldChannel,
		nil,
		thesis,
	)
	resonanceSolver := resonance.NewSolver(
		systemCtx,
		viper.GetFloat64("resonance.learning_rate"),
		api,
		uiChannel,
		thesis,
	)
	crypto, err := trader.NewCrypto(
		systemCtx,
		api,
		uiChannel,
		manifoldChannel,
		nil,
		desk,
		thesis,
	)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "boot: create crypto", err))
		cancel()
		return nil
	}

	planner := strategy.NewPlanner(systemCtx, thesis, recorder, desk)
	planner.ObserveModule = crypto.ObserveModule()
	planner.ObserveHop = crypto.ObserveHop()
	existingHub := hub != nil

	if hub == nil {
		hub = ui.NewHub(systemCtx, thesis, desk, price, balance, manifoldChannel)
	}

	attachDiagnosticsErrorBridge(hub, crypto)
	systems := []Runnable{
		api,
	}

	if existingHub == false {
		systems = append(systems, hub)
	}

	closers := []func() error{
		regulatorSolver.Close,
		func() error {
			api.Close()
			return nil
		},
	}

	for _, signal := range signals {
		closers = append(closers, signal.Close)
	}

	closers = append(
		closers,
		planner.Close,
		resonanceSolver.Close,
		analyzer.Close,
		crypto.Close,
		desk.Close,
		hub.Close,
	)

	stack := &System{
		ctx:       systemCtx,
		cancel:    cancel,
		Hub:       hub,
		Desk:      desk,
		Planner:   planner,
		Analyzer:  analyzer,
		Crypto:    crypto,
		Regulator: regulatorSolver,
		Signals:   signals,
		Thesis:    thesis,
		Systems:   systems,
		closers:   closers,
		resources: []func() error{positionStore.Close},
		runDone:   make(chan struct{}),
	}
	thesis.SetFailureHandler(stack.fail)

	if err := instrument.Subscribe(); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"boot: subscribe instruments",
			err,
		))
		cancel()
		return nil
	}

	return stack
}

/*
diagnosticsBridgeOnce ensures the error bridge is attached to the global logger
only once per process, even though Boot may run many times in the test suite.
The bridge feeds subsystem-attributed errors into the diagnostics WebRTC frame.
*/
var diagnosticsBridgeOnce sync.Once

func attachDiagnosticsErrorBridge(hub *ui.Hub, crypto *trader.Crypto) {
	if hub == nil || crypto == nil {
		return
	}

	diagnosticsBridgeOnce.Do(func() {
		errnie.AttachWriter(ui.NewErrorBridge(
			hub,
			nil,
			crypto.ObserveDiagnosticError,
		))
	})
}
