package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"

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
Holding reports how many open lots the desk carries for one symbol.

Inventory is the only question a caller driving the system from outside can ask
that the system answers about itself rather than about its opinions, so it is
what tells a harness that a position has actually been run out.
*/
func (system *System) Holding(symbol string) int {
	if system == nil || system.Desk == nil {
		return 0
	}

	return system.Desk.Holding(symbol)
}

/*
Close releases every resource Boot acquired, in reverse order of acquisition.
*/
func (system *System) Close() error {
	if system == nil {
		return nil
	}

	for _, v := range slices.Backward(system.closers) {
		errnie.Error(v())
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
	uiChannel chan []byte,
) *System {
	viper.SetDefault("system.actor.buffer", 1024)

	if !viper.IsSet("market.subscribe.batch") {
		if err := loadEmbeddedConfig(); err != nil {
			errnie.Error(err)
			return nil
		}
	}

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

	dataPath := utils.ResolveDataPath()

	if dataPath == "" {
		errnie.Error(fmt.Errorf("system data path required"))
		_ = recorder.Close()
		return nil
	}

	positionStore, err := broker.NewPositionStore(
		filepath.Join(dataPath, "symm.sqlite"),
	)

	if err != nil {
		errnie.Error(fmt.Errorf("failed to create position store: %w", err))
		_ = recorder.Close()
		return nil
	}

	system := &System{
		Thesis:  thesis,
		closers: []func() error{recorder.Close, positionStore.Close},
	}

	if public == nil {
		public = websocket.New(ctx, nil, false, websocket.PublicWebSocketURL)
	}

	if private == nil {
		private = websocket.New(ctx, nil, true, websocket.PrivateWebSocketURL)
	}

	api := utils.NewWaiter[*websocket.API](websocket.NewAPI(
		ctx, public, private,
	)).Wait()

	errnie.Debug("api reported to be ready")

	price := utils.NewWaiter[*broker.Price](broker.NewPrice(api)).Wait()
	errnie.Debug("price reported to be ready")

	instrument := utils.NewWaiter[*broker.Instrument](
		broker.NewInstrument(api, price, uiChannel),
	).Wait()

	errnie.Debug("instrument reported to be ready")

	balance := utils.NewWaiter[*broker.Balance](
		broker.NewBalance(api, uiChannel),
	).Wait()

	errnie.Debug("balance reported to be ready")

	desk := utils.NewWaiter[*broker.Desk](broker.NewDesk(
		ctx,
		api,
		instrument,
		price,
		balance,
		recorder,
		positionStore,
		uiChannel,
	)).Wait()

	errnie.Debug("desk reported to be ready")

	tree, err := dmt.NewTree("")

	if err != nil {
		errnie.Error(fmt.Errorf("failed to create decision tree: %w", err))
		return nil
	}

	crypto := utils.NewWaiter[*trader.Crypto](trader.NewCrypto(
		ctx,
		api,
		uiChannel,
		recorder,
		desk,
		thesis,
	)).Wait()

	errnie.Debug("trader reported to be ready")

	signalSubscriptions := map[string]*types.Subscription[any]{}

	for _, signal := range []types.Signal{
		utils.NewWaiter[*correlation.Signal](correlation.NewSignal(ctx, api, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*cvd.Signal](cvd.NewSignal(ctx, api, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*depthflow.Signal](depthflow.NewSignal(ctx, api, instrument, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*exhaust.Signal](exhaust.NewSignal(ctx, api, instrument, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*hawkes.Signal](hawkes.NewSignal(ctx, api, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*leadlag.Signal](leadlag.NewSignal(ctx, api, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*liquidity.Signal](liquidity.NewSignal(ctx, api, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*pumpdump.Signal](pumpdump.NewSignal(ctx, api, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*sentiment.Signal](sentiment.NewSignal(ctx, api, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
		utils.NewWaiter[*toxicity.Signal](toxicity.NewSignal(ctx, api, uiChannel, map[string]*types.Subscription[any]{"thesis": crypto.Subscribe("thesis", types.NewSubscription[any]())})).Wait(),
	} {
		errnie.Debug(fmt.Sprintf("%s signal reported to be ready", signal.Name()))
		signalSubscriptions[signal.Name()] = signal.Subscribe(
			signal.Name(), types.NewSubscription[any](),
		)
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

	errnie.Debug("analyzer reported to be ready")

	planner := utils.NewWaiter[*strategy.Planner](strategy.NewPlanner(
		ctx,
		uiChannel,
		analyzer,
		recorder,
		desk,
	)).Wait()

	errnie.Debug("planner reported to be ready")

	crypto.AddSubscription("planner", planner.Subscribe("planner", types.NewSubscription[any]()))

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
