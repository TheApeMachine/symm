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
	"github.com/theapemachine/symm/backtest"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/logic/resonance"
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
Sync waits until the trader has consumed every frame already delivered and the
manifold has settled the field those frames produced. Replay uses this so the
next captured arrival cannot overtake the decision that belongs to the current
one.
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
	return BootWithHub(ctx, thesis, public, private, uiChannel, nil)
}

/*
BootWithHub boots the full system reusing an already-serving hub, which the
backtest driver needs: one hub survives every seek's stack rebuild, so the
dashboard connection and playback controls never drop.
*/
func BootWithHub(
	ctx context.Context,
	thesis *types.Thesis,
	public websocket.Conn,
	private websocket.Conn,
	uiChannel chan []byte,
	existingHub *ui.Hub,
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

	// The analytical audit stream lives in sqlite beside the captures: only
	// decision moments are written, so the events table stays tiny while the
	// file recorder remains as a fallback for sessions without a store.
	captureStore, storeErr := backtest.NewStore(
		filepath.Join(dataPath, "symm.sqlite"),
	)

	if storeErr != nil {
		errnie.Error(fmt.Errorf("failed to open capture store: %w", storeErr))
		_ = recorder.Close()
		return nil
	}

	recorder.EventSink = captureStore.WriteEvent

	closers := []func() error{recorder.Close, captureStore.Close}
	var marketRecorder websocket.CaptureSink

	if public == nil || private == nil {
		captureWriter, writeErr := captureStore.OpenCapture()

		if writeErr != nil {
			errnie.Error(fmt.Errorf("failed to open capture: %w", writeErr))
			_ = recorder.Close()
			_ = captureStore.Close()
			return nil
		}

		marketRecorder = captureWriter
		closers = append(closers, captureWriter.Close)
	}

	positionStore, err := broker.NewPositionStore(
		filepath.Join(dataPath, "symm.sqlite"),
	)

	if err != nil {
		errnie.Error(fmt.Errorf("failed to create position store: %w", err))

		for _, close := range closers {
			_ = close()
		}

		return nil
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

	utils.NewWaiter[*resonance.Solver](resonance.NewSolver(
		ctx,
		viper.GetFloat64("resonance.learning_rate"),
		api,
		uiChannel,
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

	if existingHub != nil {
		system.Hub = existingHub

		existingHub.SetPlayback(nil, func() any {
			captures, listErr := captureStore.ListCaptures()

			if listErr != nil {
				return []backtest.CaptureInfo{}
			}

			return captures
		})

		errnie.Debug("reusing served hub")
	} else {
		system.Hub = ui.NewHub(
			ctx,
			desk,
			price,
			balance,
			uiChannel,
			manifoldChannel,
		)

		// Live runs have no playback driver, but the capture history is
		// still real: the dashboard lists captured sessions read-only.
		system.Hub.SetPlayback(nil, func() any {
			captures, listErr := captureStore.ListCaptures()

			if listErr != nil {
				return []backtest.CaptureInfo{}
			}

			return captures
		})

		system.closers = append(system.closers, system.Hub.Close)
	}

	system.Desk = desk
	system.Planner = planner
	system.Analyzer = analyzer
	system.Crypto = crypto
	system.Regulator = regulatorSolver

	return system
}
