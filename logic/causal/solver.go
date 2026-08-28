package causal

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/algo"
	nmcausal "github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
PearlConfig is the solver's own role mapping for the causal ladder: which
columns of the retained row are the target, the treatment, and the controls. It
is a domain config that lives in the application layer; the algo.Pearl primitive
itself has no config.
*/
type PearlConfig struct {
	Target                  int
	Treatment               int
	Controls                []int
	TreatmentInverted       int
	ControlsInverted        []int
	InterventionLevel       float64
	InterventionPercentile  float64
	NonlinearCounterfactual bool
}

/*
Option configures the Causal solver.
*/
type Option func(*Solver)

/*
WithPearlConfig customizes Pearl's causal ladder configuration.
*/
func WithPearlConfig(config PearlConfig) Option {
	return func(s *Solver) {
		s.config = config
	}
}

/*
Solver evaluates Judea Pearl's Causal Ladder over live market measurements,
physics fluid metrics, and predictive coding resonance predictions.
*/
type Solver struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	thesis        *types.Thesis
	price         *broker.Price
	recorder      *audit.Recorder
	pearls        *sync.Map
	rows          *sync.Map
	config        PearlConfig
	ObserveModule func(string, time.Duration)
}

/*
NewSolver creates a typed causal solver.
Default layout (4-column row):
  - Col 0: Control 1 (Resonance System Energy)
  - Col 1: Control 2 (Resonance Surprise / Anomaly)
  - Col 2: Treatment (Resonance Task Prediction / Expected Return)
  - Col 3: Target (Realized Price Return)
*/
func NewSolver(bus *runtime.Workspace, opts ...Option) *Solver {
	ctx, cancel := context.WithCancel(context.Background())
	defaultConfig := PearlConfig{
		Target:                  3,
		Treatment:               2,
		Controls:                []int{0, 1},
		NonlinearCounterfactual: true,
	}

	var thesis *types.Thesis
	var price *broker.Price
	if bus != nil {
		if shared, found := bus.Shared("thesis", ""); found {
			if t, ok := shared.(*types.Thesis); ok {
				thesis = t
			}
		}
		if shared, found := bus.Shared("price", ""); found {
			if p, ok := shared.(*broker.Price); ok {
				price = p
			}
		}
	}

	solver := &Solver{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		price:  price,
		pearls: &sync.Map{},
		rows:   &sync.Map{},
		config: defaultConfig,
	}

	for _, opt := range opts {
		opt(solver)
	}

	if bus != nil {
		runtime.Register(
			bus,
			nil,
			func(artifact *types.ResonanceArtifact) *types.CausalOutput {
				if artifact == nil {
					return nil
				}

				return solver.Step(*artifact)
			},
		)
	}

	return solver
}

func (solver *Solver) Name() string {
	return "causal"
}

func (solver *Solver) Error() error { return solver.err }

/*
Step evaluates one symbol's resonance stream through Pearl's causal ladder. The
transport workspace preserves order for this symbol while every other symbol's
lane advances concurrently, so one slow causal read never fences the universe.
*/
func (solver *Solver) Step(artifact types.ResonanceArtifact) *types.CausalOutput {
	if solver == nil || solver.thesis == nil || artifact.Manifold == nil {
		return nil
	}

	started := time.Now()
	defer func() {
		if solver.ObserveModule != nil {
			solver.ObserveModule("causal", time.Since(started))
		}
	}()

	out, ok, err := solver.measure(solver.thesis, artifact.Symbol, artifact.Manifold)
	solver.err = err

	if err != nil && solver.thesis != nil {
		solver.thesis.Fail(err)
		return nil
	}

	if !ok {
		return nil
	}

	return &out
}

func (solver *Solver) measure(
	thesis *types.Thesis,
	symbol string,
	coder *learning.ResonanceManifold,
) (types.CausalOutput, bool, error) {
	symbolValue, found := thesis.Symbols.Load(symbol)

	if !found {
		return types.CausalOutput{}, false, nil
	}

	symbolState := symbolValue.(*types.Symbol)

	forecast, err := coder.RolloutTaskForecast(1)

	if err != nil {
		return types.CausalOutput{}, false, errnie.Err(
			errnie.UnprocessableContent,
			"causal: resonance forecast failed: "+err.Error(),
			err,
		)
	}

	forecastReady := len(forecast) > 0 && forecast[0].Ready
	prediction := 0.0

	if forecastReady {
		prediction = forecast[0].Value
	}

	_, surprise, energy := coder.WireSnapshot()

	midpoint := 0.0
	tickerAt := thesis.At

	if solver.price != nil {
		ticker := solver.price.Tick(symbol)

		if ticker != nil {
			tickerAt = ticker.Timestamp

			if ticker.Bid != nil && ticker.Ask != nil {
				bid := ticker.Bid.Float64()
				ask := ticker.Ask.Float64()

				if bid > 0 && ask >= bid {
					midpoint = (bid + ask) / 2
				}
			}

			if midpoint == 0 && ticker.Last != nil && ticker.Last.Sign() > 0 {
				midpoint = ticker.Last.Float64()
			}
		}
	}

	if midpoint == 0 {
		return types.CausalOutput{}, false, nil
	}

	row, rows, resolved, err := solver.observe(
		symbol,
		[3]float64{energy, surprise, prediction},
		midpoint,
		tickerAt,
		forecastReady,
	)

	if err != nil {
		return types.CausalOutput{}, false, err
	}

	if !resolved {
		return types.CausalOutput{}, false, nil
	}

	output, resolved, err := solver.evaluatePearl(symbol, row, rows, surprise, energy, prediction)

	if err != nil {
		if errors.Is(err, io.EOF) {
			unresolvedOut, hasUnresolved := solver.unresolvedOutput(symbolState, tickerAt, rows, prediction)
			return unresolvedOut, hasUnresolved, nil
		}

		return types.CausalOutput{}, false, errnie.Err(
			errnie.UnprocessableContent,
			"causal: pearl evaluation failed: "+err.Error(),
			err,
		)
	}

	if !resolved {
		unresolvedOut, hasUnresolved := solver.unresolvedOutput(symbolState, tickerAt, rows, prediction)
		return unresolvedOut, hasUnresolved, nil
	}

	precision, err := solver.estimatePrecision(rows)

	if err != nil {
		return types.CausalOutput{}, false, errnie.Err(
			errnie.UnprocessableContent,
			"causal: precision estimation failed: "+err.Error(),
			err,
		)
	}

	output["symbol"] = symbolState.Symbol
	output["at"] = tickerAt
	output["historyRows"] = rows
	output["samples"] = len(rows)
	output["precision"] = precision
	output["treatmentLevel"] = prediction

	if solver != nil && solver.thesis != nil {
		solver.conditionOnManifold(solver.thesis, output)
	}

	return types.CausalOutput{Symbol: symbolState.Symbol, Rows: output}, true, nil
}

func (solver *Solver) conditionOnManifold(
	thesis *types.Thesis,
	causalOutput map[string]any,
) {
	if thesis == nil {
		return
	}

	reading, hasReading := thesis.ManifoldSnapshot()
	scores, hasScores := thesis.InterventionSnapshot()

	if !hasReading || !hasScores || len(scores) == 0 {
		return
	}

	best := scores[0]

	for _, score := range scores[1:] {
		if score.Score > best.Score {
			best = score
		}
	}

	gate := reading.KuramotoR * reading.CoherenceMag2 / (1 + math.Abs(reading.Divergence))
	uplift := best.Score * gate
	causalOutput["doExpectation"] = uplift
	causalOutput["intervention"] = uplift
	causalOutput["interventionScore"] = math.Abs(uplift)
	causalOutput["identification"] = "manifoldBVP"
	causalOutput["manifoldAction"] = best.Action
	causalOutput["treatmentLevel"] = uplift
}

func (solver *Solver) unresolvedOutput(
	symbolState *types.Symbol,
	at time.Time,
	rows [][]float64,
	prediction float64,
) (types.CausalOutput, bool) {
	if len(rows) == 0 {
		return types.CausalOutput{}, false
	}

	precision, err := solver.estimatePrecision(rows)

	if err != nil {
		return types.CausalOutput{}, false
	}

	return types.CausalOutput{Symbol: symbolState.Symbol, Rows: map[string]any{
		"symbol":         symbolState.Symbol,
		"at":             at,
		"historyRows":    rows,
		"identification": "unresolved",
		"precision":      precision,
		"samples":        len(rows),
		"treatmentLevel": prediction,
	}}, true
}

/*
estimatePrecision reports the weakest finite-sample precision required to
identify a treatment effect: both the treatment and target must vary. It uses
the same predictive-scale precision measure as nomagique's online
standardizer, so precision rises continuously instead of crossing a row-count
gate.
*/
func (solver *Solver) estimatePrecision(rows [][]float64) (float64, error) {
	treatmentPrecision := 0.0
	targetPrecision := 0.0

	for _, row := range rows {
		if solver.config.Treatment < 0 || solver.config.Treatment >= len(row) ||
			solver.config.Target < 0 || solver.config.Target >= len(row) {
			return 0, errnie.Err(
				errnie.Validation,
				"causal: treatment and target must fit retained row width",
				nil,
			)
		}
	}

	if len(rows) > 0 {
		treatmentPrecision = columnPrecision(rows, solver.config.Treatment)
		targetPrecision = columnPrecision(rows, solver.config.Target)
	}

	return math.Min(treatmentPrecision, targetPrecision), nil
}

/*
columnPrecision returns the predictive precision of one column of the retained
rows. Precision is a pure function of the observation count — the adaptive
Standardizer's precision slot is precisionFor(count), with count the number of
accepted values — so running the whole nomagique.Number + Frame pipeline per
call to reach a count-derived scalar allocated a 66 KB frame and interned
symbols on every row for no information. The equivalent result is the shared
adaptive.StandardizerPrecision primitive applied to the accepted-row count.
*/
func columnPrecision(
	rows [][]float64,
	column int,
) float64 {
	accepted := 0.0

	for _, row := range rows {
		if column < 0 || column >= len(row) {
			continue
		}

		accepted++
	}

	return adaptive.StandardizerPrecision(accepted)
}

/*
evaluatePearl runs the in-repo algo.Pearl composition over the retained row
window and returns the ladder channel readout as the map shape the causal frame
consumes, plus resolved. The retained rows are loaded into a frame (row-major
sample slots), the roles and level are declared, and the composition writes the
channels; the map is then populated from the frame.
*/
func (solver *Solver) evaluatePearl(
	symbol string,
	row []float64,
	rows [][]float64,
	contagion float64,
	condition float64,
	intervention float64,
) (map[string]any, bool, error) {
	frame := nmtypes.Frame{}
	frame.Put(nmcausal.SymbolRowCount, float64(len(rows)))
	frame.Put(nmcausal.SymbolTarget, float64(solver.config.Target))
	frame.Put(nmcausal.SymbolTreatment, float64(solver.config.Treatment))
	frame.Put(nmcausal.SymbolLevel, intervention)
	frame.Put(nmcausal.SymbolBandwidth, solveBandwidth(rows, solver.config.Controls))

	sampleIndex := 0

	for _, row := range rows {
		for _, value := range row {
			frame.Put(nmtypes.MustSampleSymbol(sampleIndex), value)
			sampleIndex++
		}
	}

	output := algo.Pearl()(frame)

	if output.Err != nil {
		return nil, false, output.Err
	}

	association, hasAssociation := output.Get(nmcausal.SymbolAssociation)
	backdoor, hasBackdoor := output.Get(nmcausal.SymbolBackdoor)
	doExpectation, hasDo := output.Get(nmcausal.SymbolDoExpectation)
	counterfactual, hasCounterfactual := output.Get(nmcausal.SymbolCounterfactual)
	noise, hasNoise := output.Get(nmcausal.SymbolNoise)
	treatmentScale, hasTreatmentScale := output.Get(nmcausal.SymbolTreatmentScale)
	targetScale, hasTargetScale := output.Get(nmcausal.SymbolTargetScale)

	if !hasAssociation || !hasBackdoor || !hasDo || !hasCounterfactual ||
		!hasNoise || !hasTreatmentScale || !hasTargetScale {
		return nil, false, nil
	}

	associationScore := math.Abs(association)
	interventionScore := math.Abs(backdoor * treatmentScale / targetScale)
	upliftScore := math.Abs(counterfactual) / targetScale
	noiseScore := math.Abs(noise) / targetScale

	winner, hasWinner := output.Get(probability.SymbolWinner)
	confidence, hasConfidence := output.Get(probability.SymbolConfidence)

	if !hasWinner || !hasConfidence {
		return nil, false, nil
	}

	return map[string]any{
		"association":       association,
		"associationScore":  associationScore,
		"intervention":      backdoor,
		"interventionScore": interventionScore,
		"doExpectation":     doExpectation,
		"uplift":            counterfactual,
		"upliftScore":       upliftScore,
		"noise":             noise,
		"noiseScore":        noiseScore,
		"residual":          noiseScore,
		"contagion":         contagion,
		"condition":         condition,
		"value":             winner,
		"category":          winner,
		"confidence":        confidence,
		"strength":          math.Max(associationScore, math.Max(interventionScore, math.Max(upliftScore, noiseScore))),
	}, true, nil
}

/*
solveBandwidth derives the kernel bandwidth for the backdoor weighting from the
retained rows using Scott's multivariate Gaussian-kernel rule on the control
dimension. When there are no controls the bandwidth is zero (no kernel
weighting); otherwise it is a data-derived spread, never a fixed constant.
*/
func solveBandwidth(rows [][]float64, controls []int) float64 {
	if len(rows) < 2 || len(controls) == 0 {
		return 0
	}

	dimension := float64(len(controls))

	return math.Pow(float64(len(rows)), -1/(dimension+4))
}

/*
Close cleans up the solver instance.
*/
func (solver *Solver) Close() error {
	solver.cancel()
	return nil
}
