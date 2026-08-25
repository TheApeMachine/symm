package cmd

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/regulator"
	"github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/strategy"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
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
	Crypto    *Crypto
	Regulator *regulator.Solver
	Runner    *signal.Runner
	Thesis    *types.Thesis
	Bus       *runtime.Workspace
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
	bus *runtime.Workspace,
) *System {
	return BootWithHub(ctx, thesis, public, private, bus, nil)
}

func BootWithHub(
	ctx context.Context,
	thesis *types.Thesis,
	public websocket.Conn,
	private websocket.Conn,
	bus *runtime.Workspace,
	hub *ui.Hub,
	recorders ...*audit.Recorder,
) *System {
	viper.SetDefault("system.actor.buffer", 1024)
	var recorder *audit.Recorder

	if len(recorders) > 0 {
		recorder = recorders[0]
	}

	if bus != nil && recorder != nil {
		bus.Share("recorder", recorder, "")
	}

	systemCtx, cancel := context.WithCancel(ctx)

	if live, ok := public.(*websocket.Live); ok {
		live.SetThesis(thesis)
		live.SetBus(bus)
	}

	if live, ok := private.(*websocket.Live); ok {
		live.SetThesis(thesis)
		live.SetBus(bus)
	}

	regulatorSolver, err := regulator.NewSolver(systemCtx, bus)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal, "boot: create regulator", err,
		))

		cancel()
		return nil
	}

	var futuresRecorders []websocket.CaptureSink

	if live, ok := public.(*websocket.Live); ok && live.Capture() != nil {
		futuresRecorders = append(futuresRecorders, live.Capture())
	} else if live, ok := private.(*websocket.Live); ok && live.Capture() != nil {
		futuresRecorders = append(futuresRecorders, live.Capture())
	}

	futures := websocket.NewFutures(systemCtx, bus, "")
	if len(futuresRecorders) > 0 {
		// we don't need to manually inject it anymore, futures gets it from bus,
		// but keeping the logic in case it's specifically needed for tests where bus is nil.
	}
	futures.SetBus(bus)
	api := websocket.NewAPI(systemCtx, public, private)
	api.SetFutures(futures)

	if bus != nil {
		bus.Share("thesis", thesis, "")
		bus.Share("api", api, "")
		bus.Share("regulator", regulatorSolver, "")
	}

	signalRunner := signal.NewRunner(systemCtx, bus)

	if bus != nil {
		runtime.WireFunc(
			bus,
			types.ChannelMeasurements,
			types.ChannelUI,
			func(value any) *types.UIFrame {
				var measurement *nmtypes.Measurement

				if m, ok := value.(*nmtypes.Measurement); ok {
					measurement = m
				} else if m, ok := value.(*data.Measurement[float64]); ok && m != nil {
					measurement = m.ToTypesMeasurement()
				}

				if measurement == nil {
					return nil
				}

				wireRow := measurementWire(measurement)

				if wireRow == nil {
					return nil
				}

				return &types.UIFrame{
					Type: wire.FrameMeasurementsFrame,
					Value: &wire.MeasurementsFrameT{
						Rows: []*wire.MeasurementT{wireRow},
					},
				}
			},
		)

		// Workspace observer side-effects own the dashboard UI frames for the
		// derived stages. Logic solvers emit domain data only; these taps convert
		// it to wire frames so UI publishing lives in one place, not in each
		// module's data path.
		runtime.WireFunc(
			bus,
			types.ChannelCausal,
			types.ChannelUI,
			func(value any) *types.UIFrame {
				var rows map[string]any
				if output, ok := value.(types.CausalOutput); ok {
					rows = output.Rows
				} else if ptr, ok := value.(*types.CausalOutput); ok && ptr != nil {
					rows = ptr.Rows
				} else {
					return nil
				}

				return &types.UIFrame{
					Type: wire.FrameCausalFrame,
					Value: &wire.CausalFrameT{
						Rows: []*wire.CausalT{causal.CausalWire(rows)},
					},
				}
			},
		)

		runtime.WireFunc(
			bus,
			types.ChannelCognition,
			types.ChannelUI,
			func(value any) *types.UIFrame {
				var reading types.Cognition
				if r, ok := value.(types.Cognition); ok {
					reading = r
				} else if ptr, ok := value.(*types.Cognition); ok && ptr != nil {
					reading = *ptr
				} else {
					return nil
				}

				return &types.UIFrame{
					Type: wire.FrameCognitionFrame,
					Value: &wire.CognitionFrameT{
						Rows: []*wire.CognitionT{cognition.CognitionWire(reading)},
					},
				}
			},
		)

		runtime.WireFunc(
			bus,
			types.ChannelResonance,
			types.ChannelUI,
			func(value any) *types.UIFrame {
				var artifact types.ResonanceArtifact
				if a, ok := value.(types.ResonanceArtifact); ok {
					artifact = a
				} else if ptr, ok := value.(*types.ResonanceArtifact); ok && ptr != nil {
					artifact = *ptr
				} else {
					return nil
				}

				return &types.UIFrame{
					Type: wire.FrameResonanceFrame,
					Value: &wire.ResonanceFrameT{
						Rows: []*wire.ResonanceT{
							resonance.ResonanceWire(artifact.Symbol, artifact.At, artifact),
						},
					},
				}
			},
		)

		runtime.WireFunc(
			bus,
			types.ChannelRelations,
			types.ChannelUI,
			func(value any) *types.UIFrame {
				var update graph.GraphUpdate
				if u, ok := value.(graph.GraphUpdate); ok {
					update = u
				} else if ptr, ok := value.(*graph.GraphUpdate); ok && ptr != nil {
					update = *ptr
				} else {
					return nil
				}

				if update.Symbol != types.Focus() {
					return nil
				}

				shared, found := bus.Shared(graph.SharedInfluenceGraph, "")

				if !found {
					return nil
				}

				influence, ok := shared.(*graph.InfluenceGraph)

				if !ok {
					return nil
				}

				return &types.UIFrame{
					Type:  wire.FrameGraphFrame,
					Value: graph.InfluenceGraphWire(influence, update.Symbol, update.At),
				}
			},
		)

		runtime.WireFunc(
			bus,
			types.ChannelDecisions,
			types.ChannelUI,
			func(value any) *types.UIFrame {
				var round types.StrategyRound
				if r, ok := value.(types.StrategyRound); ok {
					round = r
				} else if ptr, ok := value.(*types.StrategyRound); ok && ptr != nil {
					round = *ptr
				} else {
					return nil
				}

				if recorder != nil {
					_ = audit.RecordAs(recorder, audit.DecisionBatchEvent, round.Decisions)
				}

				rows := make([]*wire.DecisionT, 0, len(round.Decisions))

				for _, decision := range round.Decisions {
					if decision == nil {
						continue
					}

					rows = append(rows, types.DecisionWire(
						*decision,
						strategy.StrategyWireBranchCount,
						false,
					))
				}

				return &types.UIFrame{
					Type: wire.FrameStrategyFrame,
					Value: &wire.StrategyFrameT{
						Evaluated: round.Evaluated,
						Outcome:   round.Outcome,
						Decisions: rows,
					},
				}
			},
		)

		runtime.WireFunc(
			bus,
			types.ChannelRegulator,
			types.ChannelUI,
			func(value any) *types.UIFrame {
				var payload regulator.RegulatorPayload
				if p, ok := value.(regulator.RegulatorPayload); ok {
					payload = p
				} else if ptr, ok := value.(*regulator.RegulatorPayload); ok && ptr != nil {
					payload = *ptr
				} else {
					return nil
				}

				return &types.UIFrame{
					Type:  wire.FrameRegulatorFrame,
					Value: regulator.RegulatorWire(payload),
				}
			},
		)
	}

	price := broker.NewPrice(systemCtx, bus)

	if bus != nil {
		bus.Share("price", price, "")
	}
	instrument := broker.NewInstrument(systemCtx, bus)
	if bus != nil {
		bus.Share("instrument", instrument, "")
	}

	balance := broker.NewBalance(systemCtx, bus)
	if bus != nil {
		bus.Share("balance", balance, "")
	}

	positionStore, err := broker.NewPositionStore(
		filepath.Join(utils.ResolveDataPath(), "symm.sqlite"),
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal, "boot: open position store", err,
		))

		cancel()
		return nil
	}

	if bus != nil {
		bus.Share("positionStore", positionStore, "")
	}

	desk := broker.NewDesk(
		systemCtx,
		bus,
	)

	if bus != nil {
		bus.Share("desk", desk, "")
	}

	resonanceSolver := resonance.NewSolver(
		systemCtx,
		viper.GetFloat64("resonance.learning_rate"),
		bus,
	)

	crypto, err := NewCrypto(
		systemCtx,
		bus,
	)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal, "boot: create crypto", err,
		))

		cancel()
		return nil
	}

	signalRunner.ObserveModule = crypto.ObserveModule()

	// The Influence Graph stage subscribes to ChannelMeasurements, maintains the
	// shared Influence Graph in place, and publishes a GraphUpdate per symbol on
	// ChannelRelations — which the boot observer above projects into the
	// dashboard graph frame for the focused symbol.
	graphSolver := graph.NewSolver(
		systemCtx,
		bus,
		1,
		512,
		strategy.RelationPlansFromSchema(strategy.DefaultCausalSchema(1, time.Second), 1, 30*time.Second),
		strategy.DefaultCausalSchema(1, time.Second).Version,
	)

	manifoldSolver := manifold.NewSolver(systemCtx, bus)

	planner := strategy.NewPlanner(systemCtx, thesis, recorder, desk, bus)
	planner.ObserveModule = crypto.ObserveModule()
	planner.ObserveHop = crypto.ObserveHop()
	existingHub := hub != nil

	if hub == nil {
		hub = ui.NewHub(systemCtx, thesis, desk, price, balance, bus)
	}

	observer := audit.NewConcurrentObserver(
		planner.Stager(),
		&bootPriceAdapter{price: price},
		regulatorSolver,
	)

	go observer.Run(systemCtx)

	attachDiagnosticsErrorBridge(hub, crypto)
	if hub != nil {
		hub.SetDiagnosticsControl(crypto)
	}
	systems := []Runnable{
		api,
		futures,
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
		signalRunner.Close,
		graphSolver.Close,
		manifoldSolver.Close,
		planner.Close,
		resonanceSolver.Close,
		crypto.Close,
		desk.Close,
		hub.Close,
	}

	stack := &System{
		ctx:       systemCtx,
		cancel:    cancel,
		Hub:       hub,
		Desk:      desk,
		Planner:   planner,
		Crypto:    crypto,
		Regulator: regulatorSolver,
		Runner:    signalRunner,
		Thesis:    thesis,
		Bus:       bus,
		Systems:   systems,
		closers:   closers,
		resources: []func() error{positionStore.Close},
		runDone:   make(chan struct{}),
	}

	thesis.SetFailureHandler(stack.fail)

	if bus != nil {
		bus.SetFailureHandler(thesis.Fail)
	}

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

type bootPriceAdapter struct {
	price *broker.Price
}

func (a *bootPriceAdapter) Mark(symbol string, direction string) float64 {
	dir := broker.BUY

	if direction == "sell" {
		dir = broker.SELL
	}

	m := a.price.Mark(symbol, dir)

	if m == nil {
		return 0
	}

	f := m.Float64()

	return f
}

/*
diagnosticsBridgeOnce ensures the error bridge is attached to the global logger
only once per process, even though Boot may run many times in the test suite.
The bridge feeds subsystem-attributed errors into the diagnostics WebRTC frame.
*/
var diagnosticsBridgeOnce sync.Once

func attachDiagnosticsErrorBridge(hub *ui.Hub, crypto *Crypto) {
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

func measurementWire(measurement *nmtypes.Measurement) *wire.MeasurementT {
	if measurement == nil {
		return nil
	}

	metrics := make([]*wire.MetricT, 0, len(measurement.Metrics))

	for name, metric := range measurement.Metrics {
		if metric == nil {
			continue
		}

		normalized := 0.0
		hasNormalized := false

		if metric.Normalized != nil {
			normalized = *metric.Normalized
			hasNormalized = true
		}

		metrics = append(metrics, &wire.MetricT{
			Name:          name,
			Raw:           metric.Raw,
			Normalized:    normalized,
			HasNormalized: hasNormalized,
			Unit:          metric.Unit.String(),
		})
	}

	metadata := make([]*wire.NamedNumberT, 0, len(measurement.Metadata))

	for name, val := range measurement.Metadata {
		metadata = append(metadata, &wire.NamedNumberT{
			Name:  name,
			Value: val,
		})
	}

	var fromNs int64

	if !measurement.ObservedFrom.IsZero() {
		fromNs = measurement.ObservedFrom.UnixNano()
	}

	return &wire.MeasurementT{
		Id:           measurement.ID,
		Source:       measurement.Source,
		Symbol:       measurement.Symbol,
		At:           measurement.At.UnixNano(),
		ObservedFrom: fromNs,
		Maturity:     measurement.Maturity,
		Metrics:      metrics,
		Metadata:     metadata,
	}
}
