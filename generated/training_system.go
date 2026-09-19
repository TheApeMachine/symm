package generated

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/cognition"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
)

/*
Dependencies contains the injected live resources required by the generated training system.
*/
type Dependencies struct {
	Context context.Context
	Config  *system.Config
	Catalog *tables.Catalog
}

/*
TrainingSystem represents the compiled, orchestrated system running the nomagique pipelines.
*/
type TrainingSystem struct {
	*nmruntime.System
	deps       Dependencies
	hub        *ui.Hub
	public     *websocket.Live
	private    *websocket.Live
	futures    *websocket.FuturesLive
	api        *websocket.API
	desk       *broker.Desk
	trader     *strategy.Trader
	instrument *broker.Instrument
	price      *broker.Price
	balance    *broker.Balance
}

/*
NewTrainingSystem instantiates and recursively wires the compiled nomagique pipeline into a live TrainingSystem.
*/
func NewTrainingSystem(deps Dependencies) (*TrainingSystem, error) {
	ctx := deps.Context

	// 1. Build the compiled nomagique Number pipeline
	pipeline := NewSystem()

	// 2. Initialize the execution workspace with the compiled pipeline
	workspace := nmruntime.NewWorkspaceWithPipeline(ctx, "nomagique", pipeline)

	// 3. Connect transports
	public := websocket.New(
		ctx,
		websocket.NewSimulator(),
		false,
		deps.Config.WebSocket.Endpoints.Public,
		nil,
	)

	private := websocket.New(
		ctx,
		websocket.NewSimulator(),
		true,
		deps.Config.WebSocket.Endpoints.Private,
		nil,
	)

	futures := websocket.NewFutures(
		ctx,
		deps.Config.WebSocket.Endpoints.Futures,
		nil,
	)

	api := websocket.NewAPI(ctx, public, private, futures)

	desk := broker.NewDesk(ctx, api)
	trader := strategy.NewTraderWithWorkspace(ctx, api, workspace)

	feedPipeline := func(in any) any {
		trader.Workspace().Next(in)
		return nil
	}

	public.Connect(feedPipeline)
	private.Connect(feedPipeline)
	futures.Connect(feedPipeline)

	// 4. UI & telemetry output boundary
	hub := ui.NewHub(ctx, nil, deps.Catalog, trader, desk)
	ui.NewWebRTC(ctx, hub, nil, nil)

	trader.OnEvaluation(func(eval cognition.Evaluation) {
		hub.BroadcastEvaluation(eval)
	})

	go func() {
		if err := hub.Run(); err != nil {
			errnie.Error(err)
		}
	}()

	hub.Transition(nmruntime.READY)

	// 5. Broker state
	instrument := broker.NewInstrument(api)
	price := broker.NewPrice(ctx, api, instrument)
	balance := broker.NewBalance(ctx, api)

	if err := instrument.Error(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"symm: instrument registry failed during construction",
			err,
		))
	}

	if err := price.GetFees(instrument.Symbols()); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"[symm] initial fees are not available",
			nil,
		))
	}

	if price.Status() != nmruntime.READY {
		return nil, errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"[symm] initial price fees are not ready",
			nil,
		))
	}

	if balance.Status() != nmruntime.READY {
		return nil, errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"[symm] initial balance is not ready",
			nil,
		))
	}

	if err := instrument.Subscribe(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"symm: subscribe to instrument universe",
			err,
		))
	}

	if instrument.Status() != nmruntime.READY {
		return nil, errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"symm: instrument universe is not seeded",
			nil,
		))
	}

	for _, connection := range private.Connections() {
		connection.Transition(nmruntime.READY)
	}

	for _, transport := range []nmruntime.RuntimeSystem{hub, public, private, futures} {
		transport.Transition(nmruntime.READY)
	}

	sys := &TrainingSystem{
		System:     nmruntime.NewSystem(ctx, "training_system"),
		deps:       deps,
		hub:        hub,
		public:     public,
		private:    private,
		futures:    futures,
		api:        api,
		desk:       desk,
		trader:     trader,
		instrument: instrument,
		price:      price,
		balance:    balance,
	}

	return sys, nil
}

/*
Run executes the training system until context cancellation.
*/
func (s *TrainingSystem) Run() error {
	for s.deps.Context.Err() == nil {
		time.Sleep(10 * time.Millisecond)
	}

	return nil
}
