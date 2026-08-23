package resonance

import (
nmtypes "github.com/theapemachine/symm/nomagique/types"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"golang.design/x/lockfree/lf"

	"github.com/theapemachine/nomagique/adaptive"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/transport"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
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
	err           error
	detectors     *sync.Map
	queues        *sync.Map
	schemas       *sync.Map
	standardizers *sync.Map
	states        *sync.Map
	references    *sync.Map
	pace          float64
	ui            *transport.MapReduce[*types.UIFrame]
	thesis        *types.Thesis
	work          *transport.Consumer[*types.Symbol]

	// ObserveModule is an optional diagnostics hook reporting per-step coder
	// duration so the wiring diagram can profile the resonance stage like
	// every other pipeline node.
	ObserveModule func(string, time.Duration)
}

/*
Event is one enqueued ticker observation routed to a single feature detector.
Keying the queue by symbol makes every observation that shares a coder land in
the same lock-free FIFO; a drain goroutine replays that queue serially so the
manifold is only ever touched by one goroutine at a time.
*/
type Event struct {
	symbol   string
	at       time.Time
	features []float64
}

/*
NewSolver returns a feature detection solver using the configured pace.
*/
func NewSolver(
	ctx context.Context,
	pace float64,
	api *websocket.API,
	ui *transport.MapReduce[*types.UIFrame],
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
		thesis:        thesis,
	}

	solver.work = transport.NewConsumer[*types.Symbol](solver.Name(), solver.consume)
	thesis.Work(types.SourceResonance).Register(solver.work)

	return solver
}

func (solver *Solver) Name() string {
	return "resonance"
}

func (solver *Solver) Error() error { return solver.err }

func (solver *Solver) Status() types.Status {
	return types.READY
}

func (solver *Solver) consume() {
	go func() {
		defer func() {
			solver.thesis.Fail(solver.err)
		}()

		for symbol := range solver.thesis.Work(types.SourceResonance).Drain(
			solver.work, nil,
		) {
			select {
			case <-solver.ctx.Done():
				solver.err = solver.ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			consumer := symbol.TickerConsumers[types.TickerConsumerResonance]

			if !symbol.HasTickersFor(consumer) {
				continue
			}

			for ticker := range symbol.MarketTickers(consumer) {
				if err := solver.Update(
					ticker.Symbol,
					ticker.Timestamp,
					[]float64{
						ticker.Ask.Float64(),
						ticker.Bid.Float64(),
						ticker.AskQty,
						ticker.BidQty,
						ticker.Change.Float64(),
						ticker.ChangePct,
						ticker.High.Float64(),
						ticker.Low.Float64(),
						ticker.Last.Float64(),
						ticker.Volume,
						ticker.Vwap,
					},
				); err != nil {
					solver.err = errnie.Error(errnie.Err(
						errnie.Internal,
						"resonance: detector update failed",
						err,
					))
					return
				}
			}
		}
	}()
}

func (solver *Solver) onTicker(ticker any) {
	if ticker == nil {
		return
	}

	for _, tick := range ticker.(*kraken.Ticker).Data {
		tickerQueue, _ := solver.queues.LoadOrStore(
			tick.Symbol,
			lf.NewQueue[Event](),
		)

		queue := tickerQueue.(*lf.Queue[Event])

		queue.Enqueue(Event{
			symbol: tick.Symbol,
			at:     tick.Timestamp,
			features: []float64{
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
			},
		})
	}
}

/*
Predictive coding runs on the ticker stream only. Trade and level3 carry
order-by-order book microevents; they are not the multi-timescale sensory
frame the coder settles, and their rows have no reference midpoint, so feeding
them through the full settle+learn loop would burn manifold updates for no
honest target. They are dropped here rather than enqueued.
*/
func (solver *Solver) onTrade(trade any) {}

func (solver *Solver) onLevel3(level3 kraken.Level3Data) {}

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
	detector, found := solver.detectors.Load(symbolName)
	if !found {
		detector = learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
			CustomArch: []int{len(features), len(features) * 4, len(features) * 2, len(features)}, // Overcomplete dictionary with latent space
			MaxHorizon: 8,                                                                         // Multi-step forward rollouts up to t+8
			Target:     learning.DirectionalTarget(0.01),                                          // With deadband threshold
			Pace:       solver.pace,                                                               // Adaptive learning pace
			Learn:      true,
		})
		solver.detectors.Store(symbolName, detector)
	}

	coder, ok := detector.(*learning.PredictiveCoder)

	if !ok || coder == nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			fmt.Sprintf("resonance: detector step failed for %s", symbolName),
			nil,
		))
	}

	midpoint := midpointOrLast(features)

	standardized := solver.standardize(symbolName, features)

	hasReference := false

	if prior, found := solver.references.Load(symbolName); found {
		priorMidpoint, valid := prior.(float64)
		hasReference = valid && priorMidpoint > 0
	}

	stepStarted := time.Now()

	out, err := coder.Step(learning.PredictiveInput{
		Features:     standardized,
		Reference:    midpoint,
		HasReference: hasReference,
		Step:         symbol.Tick,
		Time:         float64(at.UnixNano()) / 1e9,
	})

	if solver.ObserveModule != nil {
		solver.ObserveModule("resonance", time.Since(stepStarted))
	}

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

	solver.publishReturns(symbol, coder, out)

	solver.thesis.Publish(&wire.FrameT{
		Type: wire.FrameResonanceFrame,
		Value: &wire.ResonanceFrameT{
			Rows: []*wire.ResonanceT{
				solver.resonanceWire(symbolName, at, coder, out),
			},
		},
	})

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
standardize z-scores one symbol's feature row per feature, using Welford
moments retained on the standardizers map. Raw ticker fields span wildly
different magnitudes (price vs quantity vs percentage), so the manifold is fed
adaptive z-scores rather than heterogeneous magnitudes; the target reference
stays in raw price so delayed resolution remains wall-clock honest.
*/
func (solver *Solver) standardize(
	symbolName string,
	features []float64,
) []float64 {
	if len(features) == 0 {
		return features
	}

	loaded, _ := solver.standardizers.LoadOrStore(
		symbolName,
		make([]*adaptive.Standardizer, len(features)),
	)
	standardizers, valid := loaded.([]*adaptive.Standardizer)

	if !valid || len(standardizers) != len(features) {
		// A symbol whose feature width shifts cannot reuse the scored schema;
		// build a fresh one so the coder's own width validation reports the
		// mismatch instead of an index panic.
		standardizers = make([]*adaptive.Standardizer, len(features))
		solver.standardizers.Store(symbolName, standardizers)
	}

	standardized := make([]float64, len(features))

	for index, value := range features {
		if standardizers[index] == nil {
			standardizers[index] = adaptive.NewStandardizer()
		}

		score, err := standardizers[index].Measure(value)

		if err != nil {
			standardized[index] = 0
			continue
		}

		standardized[index] = score.Value
	}

	return standardized
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
	coder *learning.PredictiveCoder,
	out learning.PredictiveOutput,
) {
	if symbol == nil {
		return
	}

	if coder != nil && coder.Manifold() != nil {
		symbol.Resonance.Push(coder.Manifold())
	}

	symbol.Resonance.Push(out.Dynamics)

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

	symbol.Resonance.Push(
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
resonanceWire assembles the full per-symbol resonance frame the dashboard
predictive-coding panel renders. It mirrors the frontend ResonanceFrame schema:
the per-layer state/prediction vectors from the manifold's wire snapshot, the
supervised return head's forward curve and posterior, and the physical dynamics
frame as named scalars.
*/
func (solver *Solver) resonanceWire(
	symbolName string,
	at time.Time,
	coder *learning.PredictiveCoder,
	out learning.PredictiveOutput,
) *wire.ResonanceT {
	manifold := coder.Manifold()
	layers, surprise, energy := manifold.WireSnapshot()

	wireLayers := make([]*wire.ResonanceLayerT, 0, len(layers))

	for _, layer := range layers {
		wireLayers = append(wireLayers, &wire.ResonanceLayerT{
			State: layer.State, Prediction: layer.Prediction,
			ErrorNorm: layer.ErrorNorm, Temporal: layer.Temporal,
		})
	}

	skill, skillReady := manifold.TaskSkill()
	precision, precisionReady := manifold.TaskPrecision()

	forecast := &wire.ResonanceForecastT{
		ForwardCurve: out.ForwardCurve, ForwardRetention: out.ForwardRetention,
		SupportedHorizon: int64(out.SupportedHorizon), ProbeHorizon: int64(out.SupportedHorizon),
	}

	if horizon := max(1, out.SupportedHorizon); horizon > 0 {
		rollouts, err := manifold.RolloutTaskForecast(horizon)

		if err == nil && len(rollouts) > 0 {
			posterior := make([]*wire.PosteriorT, 0, len(rollouts))

			for _, roll := range rollouts {
				posterior = append(posterior, &wire.PosteriorT{
					Value: roll.Value, Scale: roll.Scale,
					DegreesOfFreedom: roll.DegreesOfFreedom, Ready: roll.Ready,
				})
			}

			forecast.Posterior = posterior
		}
	}

	call := 0.0

	if len(out.ForwardCurve) > 0 {
		first := out.ForwardCurve[0]

		if first > 0 {
			call = 1
		} else if first < 0 {
			call = -1
		}
	}

	resolved := 0.0

	if out.LastResolution != nil {
		resolved = out.LastResolution.Target
	}

	lastError := 0.0

	if out.LastResolution != nil {
		lastError = out.LastResolution.Error
	}

	calibration := "calibrating"

	if out.Calibrated {
		calibration = "calibrated"
	}

	skillStatus := "calibrating"

	if skillReady {
		switch {
		case skill > 1:
			skillStatus = "above baseline"
		case skill == 1:
			skillStatus = "baseline"
		case skill >= 0.5:
			skillStatus = "baseline"
		default:
			skillStatus = "below baseline"
		}
	}

	return &wire.ResonanceT{
		Source: string(types.SourceResonance), Symbol: symbolName, At: at.UnixNano(),
		Samples: int64(out.ResolvedSteps), TaskRelativePrecision: precision,
		TaskRelativePrecisionReady: precisionReady, TaskCalibration: calibration,
		TaskSkill: skill, TaskSkillReady: skillReady, TaskSkillStatus: skillStatus,
		LastResolvedForecast: resolved, LastRealizedReturn: resolved,
		LastForecastError: lastError, Observables: out.Readout,
		Latent: manifold.LatentState(), Embedding: manifold.LatentState(),
		Layers: wireLayers, Energy: energy, Surprise: surprise, Forecast: forecast,
		Dynamics: dynamicsWire(out.Dynamics),
		Verdict: &wire.ResonanceVerdictT{
			Learning: "observing", Tuning: "recursive least squares",
			LearningHealth: precision, TuningHealth: precision,
			Direction: call, Conviction: out.Confidence,
		},
	}
}

/*
dynamicsWire extracts the physical predictive dynamics frame into named scalars
matching the frontend ResonanceDynamics schema.
*/
func dynamicsWire(dynamics nmtypes.Frame) *wire.ResonanceDynamicsT {
	value := func(symbol nomagique.Symbol) float64 {
		reading, _ := dynamics.Get(symbol)
		return reading
	}

	return &wire.ResonanceDynamicsT{
		Ready: value(learning.SymbolDynamicsReady), DeltaTime: value(learning.SymbolDynamicsDeltaTime),
		Position: value(learning.SymbolDynamicsPosition), Velocity: value(learning.SymbolDynamicsVelocity),
		Acceleration: value(learning.SymbolDynamicsAcceleration), Memory: value(learning.SymbolDynamicsMemory),
		MemoryScale: value(learning.SymbolDynamicsMemoryScale), StoredEnergy: value(learning.SymbolDynamicsStoredEnergy),
		SuppliedPower: value(learning.SymbolDynamicsSuppliedPower), Dissipation: value(learning.SymbolDynamicsDissipation),
		PassivityResidue: value(learning.SymbolDynamicsPassivityResidue), ContinuousVariance: value(learning.SymbolDynamicsContinuousVariance),
		JumpAmplitude: value(learning.SymbolDynamicsJumpAmplitude), JumpVariance: value(learning.SymbolDynamicsJumpVariance),
		SampleCount: value(learning.SymbolDynamicsSampleCount), RotorScalar: value(learning.SymbolDynamicsRotorScalar),
		RotorBivector: value(learning.SymbolDynamicsRotorBivector), EquivarianceNorm: value(learning.SymbolDynamicsEquivarianceNorm),
	}
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
