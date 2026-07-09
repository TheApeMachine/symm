package logic

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
idealGasGamma models an ideal monatomic gas:
atoms with only translational degrees of freedom.
*/
const idealGasGamma = 5.0 / 3.0

/*
defaultBaselineHalflife is the fallback decay half-life for per-metric baseline
normalization when market.baseline_halflife is unset.
*/
const defaultBaselineHalflife = 30 * time.Second

/*
baselineEpsilon is the additive floor in the time-elastic sample/baseline ratio.
*/
const baselineEpsilon = 1e-9

type Manifold struct {
	thesis      *strategy.Thesis
	config      *pmanifold.Config
	solver      *pmanifold.Solver
	oscillators []pmanifold.Oscillator
	tree        *dmt.Tree
	scratch     dmt.ClassificationScratch
	sequence    []byte
	surprisals  []dmt.SurprisalItem
	classes     dmt.ClassificationResult
	lookahead   []dmt.LookaheadPrediction
	halflife    time.Duration
	baselines   map[string]*adaptive.TimeElastic
}

func NewManifold(thesis *strategy.Thesis, tree *dmt.Tree) *Manifold {
	bookDepth := viper.GetViper().GetInt("market.l3_depth")

	halflife := viper.GetViper().GetDuration("market.baseline_halflife")

	if halflife <= 0 {
		halflife = defaultBaselineHalflife
	}

	config := &pmanifold.Config{
		GridX:    uint32(bookDepth),
		GridY:    uint32(len(types.CategoryOrder)),
		GridZ:    uint32(len(analyzerSources)),
		DomainX:  float64(bookDepth),
		DomainY:  float64(len(types.CategoryOrder)),
		DomainZ:  float64(len(analyzerSources)),
		DeltaT:   types.Unit,
		Gamma:    idealGasGamma,
		MaxModes: uint32(len(types.CategoryOrder)),
	}

	// Derive the thermodynamic floors (c_v, rho_min, p_min, gas envelope,
	// k_thermal) the gas/GPE kernels require. Without this the solver runs with
	// a degenerate, zero-floor equation of state.
	pmanifold.ApplyDerivedGasParams(config)

	manifold := &Manifold{
		thesis:      thesis,
		solver:      pmanifold.NewSolver(*config),
		config:      config,
		oscillators: make([]pmanifold.Oscillator, len(types.CategoryOrder)),
		tree:        tree,
		halflife:    halflife,
		baselines:   make(map[string]*adaptive.TimeElastic),
	}

	if manifold.tree == nil {
		manifold.tree = dmt.NewTree("")
	}

	for index := range types.CategoryOrder {
		manifold.oscillators[index] = pmanifold.Oscillator{
			Phase:     0,
			Omega:     types.Unit,
			Amplitude: types.Unit,
			PosX:      float64(float64(config.GridX) - float64(1)/2),
			PosY:      float64(index),
			PosZ:      float64(float64(config.GridZ) - float64(1)/2),
			Heat:      types.Unit,
		}
	}

	if err := manifold.solver.SetOscillators(manifold.oscillators); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic analyzer: failed to set category oscillators",
			err,
		))
	}

	return manifold
}

/*
Close releases the GPU-backed physics solver the manifold owns.
*/
func (manifold *Manifold) Close() {
	manifold.solver.Close()
}

/*
Update turns measurements into particles that "surf" on the phase-directed pilot-wave
driven by the oscillator field underneath the compressed gas fluid.
*/
func (manifold *Manifold) Update(
	measurements []*types.Measurement,
) *strategy.Thesis {
	symbol := ""

	for _, measurement := range measurements {
		measurementSymbol := strings.TrimSpace(measurement.Symbol)

		if measurementSymbol == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic manifold: measurement symbol required",
				nil,
			))

			return manifold.thesis
		}

		if symbol == "" {
			symbol = measurementSymbol
			continue
		}

		if symbol != measurementSymbol {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic manifold: mixed-symbol batch rejected",
				nil,
			))

			return manifold.thesis
		}
	}

	if symbol != "" {
		manifold.thesis.AddEvidence("symbol", symbol)
	}

	priceSum := 0.0
	priceCount := 0
	var priceAt time.Time

	for _, measurement := range measurements {
		if price, ok := measurement.Metrics["price"]; ok && !math.IsNaN(price) && !math.IsInf(price, 0) {
			priceSum += price
			priceCount++

			if !measurement.At.IsZero() &&
				(priceAt.IsZero() || measurement.At.After(priceAt)) {
				priceAt = measurement.At
			}
		}

		// Z is the source axis (one slice per signal stream); it is a static
		// index, not a metric-driven coordinate.
		sourceIndex := slices.Index(analyzerSources, measurement.Source)

		if sourceIndex < 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic analyzer: unknown measurement source",
				nil,
			))

			continue
		}

		mapping, ok := analyzerMetrics[measurement.Source][measurement.Stream]

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic manifold: no deposit mapping for source/stream "+string(measurement.Source)+"/"+measurement.Stream,
				nil,
			))

			continue
		}

		cellXValue, okX := measurement.Metrics[mapping["cellX"]]
		momXValue, okMomX := measurement.Metrics[mapping["momX"]]
		momYValue, okMomY := measurement.Metrics[mapping["momY"]]
		momZValue, okMomZ := measurement.Metrics[mapping["momZ"]]

		if !okX || !okMomX || !okMomY || !okMomZ {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic manifold: measurement missing a mapped deposit metric",
				nil,
			))

			continue
		}

		// Metrics arrive on wildly different scales across sources (reynolds ~1e5,
		// sourceBalance ~[-1,1], spread in price units). Normalize each through a
		// per-metric time-decayed baseline so the spatial coordinate and momentum
		// components are commensurable O(1) deviations from their own recent
		// history. Until every baseline is ready the measurement is skipped rather
		// than deposited raw, which would defeat the normalization.
		keyPrefix := string(measurement.Source) + "/" + measurement.Stream + "/"

		normX, readyX := manifold.normalize(keyPrefix+"cellX", cellXValue, measurement.At)
		momX, readyMomX := manifold.normalize(keyPrefix+"momX", momXValue, measurement.At)
		momY, readyMomY := manifold.normalize(keyPrefix+"momY", momYValue, measurement.At)
		momZ, readyMomZ := manifold.normalize(keyPrefix+"momZ", momZValue, measurement.At)

		if !readyX || !readyMomX || !readyMomY || !readyMomZ {
			continue
		}

		// X is the only free spatial axis: squash the normalized coordinate into
		// (0,1) before projecting onto the axis so it spreads across the grid
		// instead of piling on the boundary cell.
		squashed := 1.0 / (1.0 + math.Exp(-normX))
		cellX := uint32(math.Min(float64(manifold.config.GridX-1),
			math.Max(0, math.Floor(squashed*float64(manifold.config.GridX)))))

		if math.IsNaN(momX) || math.IsInf(momX, 0) ||
			math.IsNaN(momY) || math.IsInf(momY, 0) ||
			math.IsNaN(momZ) || math.IsInf(momZ, 0) {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic manifold: non-finite deposit momentum",
				nil,
			))

			continue
		}

		cellZ := uint32(sourceIndex)

		// Surprisal is derived, not trusted from the signal: encode this
		// measurement's classified categories as an underscore-delimited sequence
		// of category indices and score each token's information-theoretic
		// surprisal against the tree, then fold the observation back in so future
		// surprisal reflects the history of category sequences seen.
		tokens := make([]string, 0, len(measurement.Categories))

		for _, category := range measurement.Categories {
			tokens = append(tokens, strconv.Itoa(types.CategoryIndex(category.Type)))
		}

		sequence := []byte(strings.Join(tokens, "_"))
		surprisals := manifold.tree.GetSurprisal(sequence)
		manifold.sequence = append(manifold.sequence[:0], sequence...)
		manifold.surprisals = append(manifold.surprisals[:0], surprisals...)

		if len(sequence) > 0 {
			if _, _, err := manifold.tree.UnsupervisedLearn(sequence, &manifold.scratch); err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"logic manifold: failed to learn category sequence",
					err,
				))
			}

			manifold.classes = manifold.tree.Classify(sequence, &manifold.scratch)
			manifold.lookahead = manifold.tree.PredictNextTokens(
				sequence,
				manifold.lookahead[:0],
			)
		}

		// One deposit per classified category. Y is the category axis, aligned to
		// the pinned category oscillators; rho comes from the classifier's
		// confidence/strength, and eInt from the tree-derived surprisal.
		for categoryOrder, category := range measurement.Categories {
			categoryIndex := types.CategoryIndex(category.Type)

			if categoryIndex == 0 {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"logic manifold: unknown measurement category",
					nil,
				))

				continue
			}

			surprisal := 0.0

			if categoryOrder < len(surprisals) {
				surprisal = surprisals[categoryOrder].Surprisal
			}

			rho := math.Abs(category.Confidence * category.Strength)
			eInt := math.Abs(surprisal * category.Strength)

			if math.IsNaN(rho) || math.IsInf(rho, 0) ||
				math.IsNaN(eInt) || math.IsInf(eInt, 0) {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"logic manifold: non-finite deposit quantity",
					nil,
				))

				continue
			}

			if err := manifold.solver.DepositCell(
				cellX,
				uint32(categoryIndex-1),
				cellZ,
				rho,
				momX,
				momY,
				momZ,
				eInt,
			); err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"logic analyzer: failed to deposit measurement",
					err,
				))

				return manifold.thesis
			}
		}
	}

	reading, err := manifold.solver.Step()

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"logic analyzer: failed to step manifold",
			err,
		))

		return manifold.thesis
	}

	manifold.thesis.AddEvidence("manifold", reading)

	if priceCount > 0 {
		manifold.thesis.AddEvidence("price", priceSum/float64(priceCount))

		if priceAt.IsZero() {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic manifold: price timestamp required",
				nil,
			))

			return manifold.thesis
		}

		manifold.thesis.AddEvidence("price_at", priceAt)
	}

	return manifold.thesis
}

/*
normalize expresses the metric value as a deviation from its own time-decayed
baseline, returning ok=false until the baseline is ready. The tracker requires a
non-negative value and valid timestamp, so the magnitude is normalized and the
original sign reattached. A raw value is never returned: a not-ready baseline
skips the deposit rather than pushing an unnormalized, scale-dependent magnitude
into the field.
*/
func (manifold *Manifold) normalize(key string, value float64, at time.Time) (float64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || at.IsZero() {
		return 0, false
	}

	if value == 0 {
		return 0, true
	}

	tracker, ok := manifold.baselines[key]

	if !ok {
		tracker = adaptive.NewTimeElastic(adaptive.TimeElasticConfig{
			Halflife: manifold.halflife,
			Epsilon:  baselineEpsilon,
		})
		manifold.baselines[key] = tracker
	}

	output, err := tracker.Measure(adaptive.TimedValue{
		Value: math.Abs(value),
		At:    at,
	})

	if err != nil || !output.Ready {
		return 0, false
	}

	return math.Copysign(output.Value-1, value), true
}
