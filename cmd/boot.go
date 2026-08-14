package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/regulator"
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
	Hub       *ui.Hub
	Desk      *broker.Desk
	Planner   *strategy.Planner
	Analyzer  *logic.Analyzer
	Crypto    *trader.Crypto
	Regulator *regulator.Solver
	Thesis    *types.Thesis
	closers   []func() error
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
Sync waits for the streaming analytical plane to commit all ingress delivered
before the call. It is used by deterministic venue replays only.
*/
func (system *System) Sync(ctx context.Context, at time.Time) error {
	if system == nil || system.Crypto == nil {
		return fmt.Errorf("system: streaming trader required")
	}

	return system.Crypto.Sync(ctx, at)
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

	manifoldChannel := make(chan types.FluidFrame, 1024)
	dataPath := utils.ResolveDataPath()

	if dataPath == "" {
		errnie.Error(fmt.Errorf("system data path required"))
		return nil
	}

	auditPath := filepath.Join(dataPath, "runtime-audit.jsonl")

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

	var marketRecorder *audit.Recorder
	marketPath := ""

	if public == nil || private == nil {
		marketPath = filepath.Join(dataPath, "market-frames.jsonl.zst")

		if viper.GetBool("system.audit.rotate_on_boot") {
			if err := audit.Rotate(marketPath); err != nil {
				errnie.Error(fmt.Errorf("failed to rotate market capture: %w", err))
				_ = recorder.Close()
				return nil
			}
		}

		marketRecorder, err = audit.NewRecorder(marketPath)

		if err != nil {
			errnie.Error(fmt.Errorf("failed to create market capture recorder: %w", err))
			_ = recorder.Close()
			return nil
		}
	}

	thesis.Audit = recorder.Write

	if err := thesis.Audit(map[string]any{
		"channel": "orchestration",
		"type":    "boot",
		"value": map[string]any{
			"at":                  time.Now().UTC(),
			"audit_path":          auditPath,
			"market_capture_path": marketPath,
		},
	}); err != nil {
		errnie.Error(fmt.Errorf("failed to write runtime audit boot event: %w", err))

		if marketRecorder != nil {
			_ = marketRecorder.Close()
		}

		_ = recorder.Close()
		return nil
	}

	positionStore, err := broker.NewPositionStore(
		filepath.Join(dataPath, "symm.sqlite"),
	)

	if err != nil {
		errnie.Error(fmt.Errorf("failed to create position store: %w", err))

		if marketRecorder != nil {
			_ = marketRecorder.Close()
		}

		_ = recorder.Close()
		return nil
	}

	closers := []func() error{recorder.Close}

	if marketRecorder != nil {
		closers = append(closers, marketRecorder.Close)
	}

	closers = append(closers, positionStore.Close)
	system := &System{
		Thesis:  thesis,
		closers: closers,
	}

	createdTransports := make([]websocket.Conn, 0, 2)

	if public == nil {
		public = websocket.New(
			ctx, nil, false, websocket.PublicWebSocketURL, marketRecorder,
		)
		createdTransports = append(createdTransports, public)
	}

	if private == nil {
		private = websocket.New(
			ctx, nil, true, websocket.PrivateWebSocketURL, marketRecorder,
		)
		createdTransports = append(createdTransports, private)
	}

	api := utils.NewWaiter[*websocket.API](websocket.NewAPI(
		ctx, public, private,
	)).Wait()
	system.closers = append(system.closers, func() error {
		for _, connection := range createdTransports {
			connection.Close()
		}

		return nil
	})

	errnie.Debug("api reported to be ready")

	price := utils.NewWaiter[*broker.Price](broker.NewPrice(api, marketRecorder)).Wait()
	errnie.Debug("price reported to be ready")

	instrument := utils.NewWaiter[*broker.Instrument](
		broker.NewInstrument(api, price, uiChannel),
	).Wait()

	errnie.Debug("instrument reported to be ready")

	balance := utils.NewWaiter[*broker.Balance](
		broker.NewBalance(api, uiChannel),
	).Wait()

	errnie.Debug("balance reported to be ready")
	regulatorSolver, err := regulator.NewSolver(ctx, uiChannel)

	if err != nil {
		errnie.Error(fmt.Errorf("failed to create regulator: %w", err))
		return nil
	}

	errnie.Debug("regulator reported to be ready")
	system.closers = append(system.closers, regulatorSolver.Close)

	desk := utils.NewWaiter[*broker.Desk](broker.NewDesk(
		ctx,
		api,
		instrument,
		price,
		balance,
		thesis,
		regulatorSolver,
		recorder,
		positionStore,
		uiChannel,
	)).Wait()

	errnie.Debug("desk reported to be ready")
	system.closers = append(system.closers, desk.Close)

	tree, err := dmt.NewTree("")

	if err != nil {
		errnie.Error(fmt.Errorf("failed to create decision tree: %w", err))
		return nil
	}

	analyzer := utils.NewWaiter[*logic.Analyzer](logic.NewAnalyzer(
		ctx,
		price,
		api,
		tree,
		uiChannel,
		manifoldChannel,
		recorder,
		thesis,
	)).Wait()

	errnie.Debug("analyzer reported to be ready")
	system.closers = append(system.closers, analyzer.Close)

	planner := utils.NewWaiter[*strategy.Planner](strategy.NewPlanner(
		ctx,
		uiChannel,
		thesis,
		recorder,
		desk,
	)).Wait()

	errnie.Debug("planner reported to be ready")

	crypto, err := trader.NewCrypto(
		ctx,
		api,
		uiChannel,
		recorder,
		desk,
		analyzer,
		planner,
		thesis,
	)

	if err != nil {
		errnie.Error(fmt.Errorf("failed to create streaming trader: %w", err))
		return nil
	}

	errnie.Debug("trader reported to be ready")
	system.closers = append(system.closers, crypto.Close)

	system.Hub = ui.NewHub(
		ctx,
		desk,
		price,
		balance,
		uiChannel,
		manifoldChannel,
	)
	system.closers = append(system.closers, system.Hub.Close)

	system.Desk = desk
	system.Planner = planner
	system.Analyzer = analyzer
	system.Crypto = crypto
	system.Regulator = regulatorSolver

	return system
}
