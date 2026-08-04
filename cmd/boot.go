package cmd

import (
	"context"
	"fmt"
	"path/filepath"

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
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/utils"
)

/*
System is the fully booted symm stack. It exposes the components a caller
needs to drive and observe the running system, and owns the shutdown of the
resources Boot acquired.
*/
type System struct {
	Hub      *ui.Hub
	Desk     *broker.Desk
	Planner  *strategy.Planner
	Analyzer *logic.Analyzer
	Crypto   *trader.Crypto
	Thesis   *types.Thesis
	closers  []func() error
}

/*
Close releases every resource Boot acquired, in reverse order of acquisition.
*/
func (system *System) Close() error {
	if system == nil {
		return nil
	}

	for index := len(system.closers) - 1; index >= 0; index-- {
		errnie.Error(system.closers[index]())
	}

	return nil
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
) *System {
	/*
		Boot is unusable without configuration, so load it here when the
		caller has not already done so. The cobra command sets this up via
		initConfig; tests rely on this fallback.
	*/
	if !viper.IsSet("market.subscribe.batch") {
		if err := loadEmbeddedConfig(); err != nil {
			errnie.Error(err)
			return nil
		}
	}

	uiChannel := make(chan []byte, 1024)
	manifoldChannel := make(chan []byte, 1024)
	auditPath := filepath.Join(utils.ResolveDataPath(), "runtime-audit.jsonl")

	if viper.GetBool("system.audit.rotate_on_boot") {
		if err := audit.Rotate(auditPath); err != nil {
			errnie.Error(fmt.Errorf("failed to rotate runtime audit: %w", err))
			return nil
		}
	}

	recorder, err := audit.NewRecorder(auditPath)

	if err != nil {
		errnie.Error(fmt.Errorf("failed to create runtime audit recorder: %w", err))
		return nil
	}

	system := &System{Thesis: thesis, closers: []func() error{recorder.Close}}

	if public == nil {
		public = websocket.New(ctx, nil, false, websocket.PublicWebSocketURL)
	}

	if private == nil {
		private = websocket.New(ctx, nil, true, websocket.PrivateWebSocketURL)
	}

	api := utils.NewWaiter[*websocket.API](websocket.NewAPI(
		ctx, public, private,
	)).Wait()

	errnie.Info("api reported to be ready")

	price := utils.NewWaiter[*broker.Price](broker.NewPrice(api)).Wait()
	errnie.Info("price reported to be ready")

	instrument := utils.NewWaiter[*broker.Instrument](
		broker.NewInstrument(api, price, uiChannel),
	).Wait()

	errnie.Info("instrument reported to be ready")

	balance := utils.NewWaiter[*broker.Balance](
		broker.NewBalance(api, uiChannel),
	).Wait()

	errnie.Info("balance reported to be ready")

	desk := utils.NewWaiter[*broker.Desk](broker.NewDesk(
		ctx,
		api,
		instrument,
		price,
		balance,
		recorder,
		uiChannel,
	)).Wait()

	errnie.Info("desk reported to be ready")

	tree, err := dmt.NewTree("")

	if err != nil {
		errnie.Error(fmt.Errorf("failed to create decision tree: %w", err))
		return nil
	}

	planner := utils.NewWaiter[*strategy.Planner](strategy.NewPlanner(
		ctx,
		uiChannel,
		api,
		desk,
		instrument,
		price,
		balance,
		nil,
		recorder,
	)).Wait()

	errnie.Info("planner reported to be ready")

	crypto := utils.NewWaiter[*trader.Crypto](trader.NewCrypto(
		ctx,
		api,
		uiChannel,
		recorder,
		planner,
		desk,
		thesis,
	)).Wait()

	errnie.Info("trader reported to be ready")

	signalSubscriptions := map[string]*types.Subscription[any]{}

	for _, signal := range []types.Signal{
		utils.NewWaiter[*correlation.Signal](correlation.NewSignal(ctx, api, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*cvd.Signal](cvd.NewSignal(ctx, api, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*depthflow.Signal](depthflow.NewSignal(ctx, api, instrument, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*exhaust.Signal](exhaust.NewSignal(ctx, api, instrument, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*hawkes.Signal](hawkes.NewSignal(ctx, api, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*leadlag.Signal](leadlag.NewSignal(ctx, api, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*liquidity.Signal](liquidity.NewSignal(ctx, api, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*pumpdump.Signal](pumpdump.NewSignal(ctx, api, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*sentiment.Signal](sentiment.NewSignal(ctx, api, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*toxicity.Signal](toxicity.NewSignal(ctx, api, planner, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
	} {
		errnie.Info(fmt.Sprintf("%s signal reported to be ready", signal.Name()))
		signalSubscriptions[signal.Name()] = signal.Subscribe(signal.Name(), types.NewSubscription[any]())
	}

	analyzer := utils.NewWaiter[*logic.Analyzer](logic.NewAnalyzer(
		ctx,
		api,
		tree,
		uiChannel,
		manifoldChannel,
		recorder,
		signalSubscriptions,
	)).Wait()

	errnie.Info("analyzer reported to be ready")

	planner.AttachAnalyzer(analyzer)

	system.Hub = ui.NewHub(
		ctx,
		desk,
		price,
		balance,
		uiChannel,
		manifoldChannel,
	)

	system.Desk = desk
	system.Planner = planner
	system.Analyzer = analyzer
	system.Crypto = crypto

	return system
}
