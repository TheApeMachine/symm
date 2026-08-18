package resonance

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Solver runs one feature detector per symbol over the standardized measurement
stream. The detector owns the entire predictive loop — settling, learning,
and the overcomplete sparse representation — so the solver is only the loop:
readings in, features shaped, output stored on the symbol.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	detectors     *sync.Map
	schemas       *sync.Map
	standardizers *sync.Map
	states        *sync.Map
	pace          float64
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	thesis        *types.Thesis
}

/*
NewSolver returns a feature detection solver using the configured pace.
*/
func NewSolver(
	ctx context.Context,
	pace float64,
	api *websocket.API,
	ui chan []byte,
	thesis *types.Thesis,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	solver := &Solver{
		ctx:           ctx,
		cancel:        cancel,
		detectors:     &sync.Map{},
		schemas:       &sync.Map{},
		standardizers: &sync.Map{},
		states:        &sync.Map{},
		pace:          pace,
		ui:            ui,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker": api.Subscribe(
				"ticker", types.NewSubscription[any](),
			),
			"trade": api.Subscribe(
				"trade", types.NewSubscription[any](),
			),
			"level3": api.Subscribe(
				"level3", types.NewSubscription[any](),
			),
		},
		thesis: thesis,
	}

	solver.run()
	return solver
}

func (solver *Solver) Name() string {
	return "resonance"
}

func (solver *Solver) Status() types.Status {
	return types.READY
}

func (solver *Solver) run() {
	go func() {
		for {
			select {
			case <-solver.ctx.Done():
				return
			case ticker := <-solver.subscriptions["ticker"].Channel:
				solver.onTicker(ticker)
			case trade := <-solver.subscriptions["trade"].Channel:
				solver.onTrade(trade)
			case level3 := <-solver.subscriptions["level3"].Channel:
				solver.onLevel3(level3.(kraken.Level3Data))
			}
		}
	}()
}

func (solver *Solver) onTicker(ticker any) {
	if ticker == nil {
		return
	}

	for _, tick := range ticker.(*kraken.Ticker).Data {
		solver.Update(tick.Symbol, tick.Timestamp, []float64{
			tick.Ask.Float64(),
			tick.Bid.Float64(),
			tick.AskQty,
			tick.BidQty,
			tick.Change.Float64(),
			tick.ChangePct,
			tick.High.Float64(),
			tick.Low.Float64(),
			tick.Last.Float64(),
			tick.Volume,
			tick.Vwap,
		})
	}
}

func (solver *Solver) onTrade(trade any) {
	if trade == nil {
		return
	}
}

func (solver *Solver) onLevel3(level3 kraken.Level3Data) {
	if level3.Symbol == "" {
		return
	}
}

/*
Update steps one feature detector for one symbol and publishes the settled
output as the symbol-keyed resonance frame the frontend renders: the readout
representation as the latent row, with energy and surprise alongside.
*/
func (solver *Solver) Update(
	symbolName string,
	at time.Time,
	features []float64,
) error {
	symbol := solver.thesis.Symbol(symbolName)
	detector, _ := solver.detectors.LoadOrStore(
		symbolName,
		learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
			CustomArch: []int{len(features), len(features) * 4, len(features) * 2, len(features)}, // Overcomplete dictionary with latent space
			MaxHorizon: 8,                                                                         // Multi-step forward rollouts up to t+8
			Target:     learning.DirectionalTarget(0.01),                                          // With deadband threshold
			Pace:       solver.pace,                                                               // Adaptive learning pace
			Learn:      true,
		}),
	)

	coder, ok := detector.(*learning.PredictiveCoder)

	if !ok || coder == nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: detector step failed for %s", symbolName),
			nil,
		))
	}

	out, err := coder.Step(learning.PredictiveInput{
		Features: features,
		Step:     symbol.Tick,
		Time:     float64(at.UnixNano()) / 1e9,
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: detector step failed for %s", symbolName),
			err,
		))
	}

	utils.Publish(solver.ui, datura.NewMap("resonance", datura.NewMap(
		"source", types.SourceResonance,
		"symbol", symbolName,
		"at", at,
		"latent", out.Readout,
		"energy", out.Energy,
		"surprise", out.Surprise,
		"score", out.Score,
		"direction", out.Direction,
		"confidence", out.Confidence,
		"calibrated", out.Calibrated,
	)))

	return nil
}

/*
Close stops the solver context.
*/
func (solver *Solver) Close() error {
	if solver.cancel != nil {
		solver.cancel()
	}

	return nil
}
