package stack

import (
	"context"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

/*
Stack is the production trading graph after Conn injection: the same surfaces
cmd/root.go boots, whether the API was built from live sockets or mock producers.
*/
type Stack struct {
	Booter     *system.Booter
	API        *websocket.API
	Paper      *websocket.Paper
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
SignalFactory builds the signal set once Instrument exists — the same moment
root constructs signals after broker surfaces are allocated.
*/
type SignalFactory func(
	ctx context.Context,
	api *websocket.API,
	instrument *broker.Instrument,
	channel chan []byte,
) []types.Signal

/*
Options configures shared boot. Callers supply the Conn-backed API, optional
paper transport, and the signal factory used by production.
*/
type Options struct {
	Booter         *system.Booter
	Paper          *websocket.Paper
	Signals        SignalFactory
	Channel        chan []byte
	Tree           *dmt.Tree
	Thesis         *types.Thesis
	Recorder       *audit.Recorder
	UIHub          *ui.Hub
	PreflightExtra []types.StatusReporter
	Hawkes         manifold.HawkesSource
	AttachUI       func(
		booter *system.Booter,
		price *broker.Price,
		balance *broker.Balance,
		thesis *types.Thesis,
		channel chan []byte,
	) (*ui.Hub, error)
}

/*
Boot wires Price through Crypto exactly as production does after NewAPI, then
runs Booter stages so the stack is ready for Crypto.Tick / Crypto.Run.
*/
func Boot(ctx context.Context, api *websocket.API, options Options) (*Stack, error) {
	if api == nil {
		return nil, errnie.Err(errnie.Validation, "stack: api required", nil)
	}

	if options.Signals == nil {
		return nil, errnie.Err(errnie.Validation, "stack: signals required", nil)
	}

	channel := options.Channel

	if channel == nil {
		channel = make(chan []byte, 256)
	}

	booter := options.Booter

	if booter == nil {
		booter = system.NewBooter(ctx, channel)
	}

	tree := options.Tree

	if tree == nil {
		tree = dmt.NewTree("")
	}

	thesis := options.Thesis

	if thesis == nil {
		thesis = types.NewThesis(channel, nil)
	}

	price := broker.NewPrice(api)
	instrument := broker.NewInstrument(api, price, channel)
	balance := broker.NewBalance(api, nil, channel)
	desk := broker.NewDesk(api, instrument, price, balance)

	uiHub := options.UIHub

	if options.AttachUI != nil {
		hub, hubErr := options.AttachUI(booter, price, balance, thesis, channel)

		if hubErr != nil {
			return nil, errnie.Err(errnie.Internal, "stack: ui hub", hubErr)
		}

		uiHub = hub
	}

	signals := options.Signals(ctx, api, instrument, channel)

	if len(signals) == 0 {
		return nil, errnie.Err(errnie.Validation, "stack: signals factory returned none", nil)
	}

	hawkes := options.Hawkes

	if hawkes == nil {
		for _, signal := range signals {
			if source, ok := signal.(manifold.HawkesSource); ok {
				hawkes = source
				break
			}
		}
	}

	analyzer, err := logic.NewAnalyzer(ctx, booter, api, hawkes, tree, channel)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "stack: analyzer", err)
	}

	if options.Recorder != nil {
		analyzer.SetRecorder(options.Recorder)
	}

	planner := strategy.NewPlanner(
		ctx,
		channel,
		api,
		desk,
		instrument,
		price,
		balance,
		signals,
		analyzer,
		strategy.NewAllocator(ctx, balance, instrument, price),
		options.Recorder,
	)

	crypto, err := trader.NewCrypto(
		ctx,
		booter,
		api,
		price,
		balance,
		desk,
		instrument,
		analyzer,
		planner,
		tree,
		thesis,
		uiHub,
		options.Recorder,
	)

	if err != nil {
		return nil, errnie.Err(errnie.Internal, "stack: crypto", err)
	}

	stack := &Stack{
		Booter:     booter,
		API:        api,
		Paper:      options.Paper,
		Channel:    channel,
		Tree:       tree,
		Thesis:     thesis,
		Price:      price,
		Instrument: instrument,
		Balance:    balance,
		Desk:       desk,
		Analyzer:   analyzer,
		Planner:    planner,
		Crypto:     crypto,
		UIHub:      uiHub,
		Recorder:   options.Recorder,
	}

	preflight := append([]types.StatusReporter{}, options.PreflightExtra...)

	if options.Paper != nil {
		preflight = append(preflight, options.Paper)
	}

	preflight = append(preflight,
		api,
		instrument,
		balance,
		price,
		desk,
	)

	booter.AddStages(
		system.NewStage(system.StagePreflight, preflight...),
		system.NewStage(system.StageWarmup, crypto),
		system.NewStage(system.StageReady, analyzer, planner),
	)

	if err := booter.Start(); err != nil {
		_ = crypto.Close()
		return nil, errnie.Err(errnie.Internal, "stack: boot", err)
	}

	return stack, nil
}

/*
Close releases crypto and API resources owned by the stack.
*/
func (stack *Stack) Close() {
	if stack == nil {
		return
	}

	if stack.Crypto != nil {
		errnie.Error(stack.Crypto.Close())
	}

	if stack.API != nil {
		stack.API.Close()
	}
}
