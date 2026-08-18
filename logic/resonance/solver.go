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
	references    *sync.Map
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
		references:    &sync.Map{},
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

The same pass publishes the coder's manifold, its calibrated return forecast,
and its physical dynamics onto the symbol's resonance map. The downstream
graph and causal solvers read exactly these slots; without them the predictive
readiness gate can never open and the planner would remain structurally flat.
The previous midpoint is retained per symbol so `coder.Step` receives an
honest reference and the temporal ledger actually supervises the task head —
otherwise the skill posterior never calibrates regardless of how long the
stream runs.
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

	midpoint := midpointOrLast(features)

	hasReference := false

	if prior, found := solver.references.Load(symbolName); found {
		priorMidpoint, valid := prior.(float64)
		hasReference = valid && priorMidpoint > 0
	}

	out, err := coder.Step(learning.PredictiveInput{
		Features:     features,
		Reference:    midpoint,
		HasReference: hasReference,
		Step:         symbol.Tick,
		Time:         float64(at.UnixNano()) / 1e9,
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: detector step failed for %s", symbolName),
			err,
		))
	}

	if midpoint > 0 {
		solver.references.Store(symbolName, midpoint)
	}

	solver.publishReturns(symbol, symbolName, coder, out)

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
midpointOrLast extracts an honest reference price from the standardized feature
row. The ticker feature layout is [Ask, Bid, AskQty, BidQty, Change, ChangePct,
High, Low, Last, Volume, Vwap]. The midpoint of the live touch is preferred;
when that is not available the last traded price is used as the reference.
*/
func midpointOrLast(features []float64) float64 {
	if len(features) >= 9 {
		if features[0] > 0 && features[1] > 0 {
			return (features[0] + features[1]) / 2
		}

		if features[8] > 0 {
			return features[8]
		}
	}

	return 0
}

/*
publishReturns stores the manifold and its calibrated outputs on the symbol so
the graph and causal stages can build real predictive evidence. The return
forecast carries the coder's reward prediction; it is a direction readout, not
a priced return. The manifold is stored under the symbol key because that is
the slot both downstream solvers load. The forecast and dynamics are published
only once the head is calibrated, so the graph never sees a fabricated
posterior before outcomes exist.
*/
func (solver *Solver) publishReturns(
	symbol *types.Symbol,
	symbolName string,
	coder *learning.PredictiveCoder,
	out learning.PredictiveOutput,
) {
	if symbol == nil {
		return
	}

	if coder != nil && coder.Manifold() != nil {
		symbol.Resonance.Store(symbolName, coder.Manifold())
	}

	symbol.Resonance.Store(learning.PredictiveDynamicsKey, out.Dynamics)

	if !out.Calibrated {
		return
	}

	forecasts, err := coder.Manifold().RolloutTaskForecast(1)

	if err != nil || len(forecasts) == 0 {
		return
	}

	horizon := out.SupportedHorizon

	if horizon < 1 {
		horizon = 1
	}

	forecast := forecasts[0]
	call := 0.0

	if forecast.Ready {
		if forecast.Value > 0 {
			call = 1
		} else if forecast.Value < 0 {
			call = -1
		}
	}

	symbol.Resonance.Store(
		types.ResonanceReturnForecastKey,
		&types.ResonanceReturnForecast{
			Distribution:  forecast,
			Horizon:       horizon,
			CandidateCall: call,
			Call:          call,
			StableCall:    call,
		},
	)
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
