package logic

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
idealGasGamma models an ideal monatomic gas:
atoms with only translational degrees of freedom.
*/
const idealGasGamma = 5.0 / 3.0

type Manifold struct {
	thesis      *strategy.Thesis
	config      *pmanifold.Config
	solver      *pmanifold.Solver
	oscillators []pmanifold.Oscillator
	tree        *dmt.Tree
	scratch     dmt.ClassificationScratch
}

func NewManifold(thesis *strategy.Thesis) *Manifold {
	bookDepth := viper.GetViper().GetInt("market.l3_depth")

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
		tree:        dmt.NewTree(""),
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
Update turns measurements into particles that "surf" on the phase-directed pilot-wave
driven by the oscillator field underneath the compressed gas fluid.
*/
func (manifold *Manifold) Update(
	measurements []types.Measurement,
) *strategy.Thesis {
	priceSum := 0.0
	priceCount := 0

	for _, measurement := range measurements {
		if price, ok := measurement.Metrics["price"]; ok && !math.IsNaN(price) && !math.IsInf(price, 0) {
			priceSum += price
			priceCount++
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

		mapping := analyzerMetrics[measurement.Source][measurement.Stream]
		cellZ := uint32(sourceIndex)

		// X is the only free spatial axis: project the mapped auxiliary metric
		// onto it and clamp so signed/out-of-range values land on an edge cell.
		cellX := uint32(math.Min(float64(manifold.config.GridX-1),
			math.Max(0, math.Floor(measurement.Metrics[mapping["cellX"]]*float64(manifold.config.GridX)))))

		// Momentum carries directionality from the auxiliary metrics; it may be
		// signed but must be finite.
		momX := measurement.Metrics[mapping["momX"]]
		momY := measurement.Metrics[mapping["momY"]]
		momZ := measurement.Metrics[mapping["momZ"]]

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

		if len(sequence) > 0 {
			if _, _, err := manifold.tree.UnsupervisedLearn(sequence, &manifold.scratch); err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"logic manifold: failed to learn category sequence",
					err,
				))
			}
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
	}

	return manifold.thesis
}
