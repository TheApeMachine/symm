package types

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/numeric/adaptive"
)

const (
	// DefaultCategorySurpriseAlpha is the EW update rate for categorical priors
	// (~10-sample half-life for short-horizon memory).
	DefaultCategorySurpriseAlpha = 0.1
)

func validateCategorySurpriseAlpha(alpha float64) error {
	if alpha <= 0 || alpha > 1 {
		return errnie.Error(fmt.Errorf("perspectives: invalid category surprise alpha %v", alpha))
	}

	return nil
}

/*
CategorySurpriseTracker tracks how unexpected each category selection is for one
symbol using Shannon surprisal against a self-normalizing EWMA prior.
*/
type CategorySurpriseTracker struct {
	mu            sync.Mutex
	probs         map[CategoryType]float64
	snr           *adaptive.SNR
	alpha         float64
	numCategories int
}

/*
CategorySurpriseScore is the temporal state attached to one category selection.
SNR is the surprisal score. Probability is the selected category's prior before
this observation updates it. Stability is that prior competed against the uniform
1/N prior, so habitual categories carry more confidence support than flickering
or newly rare categories.
*/
type CategorySurpriseScore struct {
	SNR         float64
	Probability float64
	Stability   float64
}

func (score CategorySurpriseScore) StabilizeConfidence(confidence float64) float64 {
	return confidence * score.Stability
}

/*
NewCategorySurpriseTracker builds a tracker with a uniform prior over the allowed
categories.
*/
func NewCategorySurpriseTracker(categories []CategoryType, alpha float64) (*CategorySurpriseTracker, error) {
	if len(categories) == 0 {
		return nil, errnie.Error(errors.New("perspectives: CategorySurpriseTracker requires categories"))
	}

	if err := validateCategorySurpriseAlpha(alpha); err != nil {
		return nil, err
	}

	probs := make(map[CategoryType]float64, len(categories))
	prior := 1.0 / float64(len(categories))

	for _, category := range categories {
		probs[category] = prior
	}

	return &CategorySurpriseTracker{
		probs:         probs,
		snr:           adaptive.NewSurprisalSNR(),
		alpha:         alpha,
		numCategories: len(categories),
	}, nil
}

/*
Score returns the Z-scored surprisal of selecting category and updates the prior.
*/
func (tracker *CategorySurpriseTracker) Score(selected CategoryType) (float64, error) {
	score, err := tracker.ScoreState(selected)

	if err != nil {
		return 0, err
	}

	return score.SNR, nil
}

/*
ScoreState returns the Z-scored surprisal and confidence stability of selecting
category, then updates the prior.
*/
func (tracker *CategorySurpriseTracker) ScoreState(
	selected CategoryType,
) (CategorySurpriseScore, error) {
	if tracker == nil {
		return CategorySurpriseScore{}, errnie.Error(errors.New("perspectives: CategorySurpriseTracker nil receiver"))
	}

	if selected == CategoryTypeNone {
		return CategorySurpriseScore{}, errnie.Error(errors.New("perspectives: CategorySurpriseTracker empty category"))
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	prob, ok := tracker.probs[selected]

	if !ok {
		scale := float64(tracker.numCategories) / float64(tracker.numCategories+1)

		for category := range tracker.probs {
			tracker.probs[category] *= scale
		}

		prob = 1.0 / float64(tracker.numCategories+1)
		tracker.probs[selected] = prob
		tracker.numCategories++
	}

	scoreProb, err := tracker.scoreProbability(prob)

	if err != nil {
		return CategorySurpriseScore{}, err
	}

	surprisal := -math.Log2(scoreProb)

	snrScore, err := tracker.snr.ScoreSurprisal(surprisal)

	if err != nil {
		return CategorySurpriseScore{}, err
	}

	stability := tracker.stabilityLocked(scoreProb)

	tracker.observeLocked(selected)

	return CategorySurpriseScore{
		SNR:         snrScore,
		Probability: scoreProb,
		Stability:   stability,
	}, nil
}

func (tracker *CategorySurpriseTracker) scoreProbability(probability float64) (float64, error) {
	if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 {
		return 0, errnie.Error(fmt.Errorf(
			"perspectives: invalid category probability %v",
			probability,
		))
	}

	priorMass := tracker.alpha / float64(tracker.numCategories)
	totalPrior := priorMass * float64(tracker.numCategories)

	if priorMass <= 0 || totalPrior <= 0 {
		return 0, errnie.Error(fmt.Errorf("perspectives: invalid category prior mass %v", priorMass))
	}

	return (probability + priorMass) / (1 + totalPrior), nil
}

func (tracker *CategorySurpriseTracker) observeLocked(selected CategoryType) {
	for category := range tracker.probs {
		if category == selected {
			tracker.probs[category] = (1-tracker.alpha)*tracker.probs[category] + tracker.alpha
			continue
		}

		tracker.probs[category] = (1 - tracker.alpha) * tracker.probs[category]
	}
}

func (tracker *CategorySurpriseTracker) stabilityLocked(probability float64) float64 {
	uniform := 1.0 / float64(tracker.numCategories)

	return UnitCompetitionMargin(probability, uniform)
}

/*
CategorySurpriseField keeps an independent categorical surprise tracker per symbol.
*/
type CategorySurpriseField struct {
	mu         sync.Mutex
	trackers   map[string]*CategorySurpriseTracker
	categories []CategoryType
	alpha      float64
}

/*
NewCategorySurpriseField builds an empty per-symbol surprise field.
*/
func NewCategorySurpriseField(categories []CategoryType, alpha float64) (*CategorySurpriseField, error) {
	if len(categories) == 0 {
		return nil, errnie.Error(errors.New("perspectives: CategorySurpriseField requires categories"))
	}

	if err := validateCategorySurpriseAlpha(alpha); err != nil {
		return nil, err
	}

	return &CategorySurpriseField{
		trackers:   make(map[string]*CategorySurpriseTracker),
		categories: append([]CategoryType(nil), categories...),
		alpha:      alpha,
	}, nil
}

/*
Score returns the surprisal SNR for symbol's category selection.
*/
func (field *CategorySurpriseField) Score(symbol string, selected CategoryType) (float64, error) {
	score, err := field.ScoreState(symbol, selected)

	if err != nil {
		return 0, err
	}

	return score.SNR, nil
}

/*
ScoreState returns the surprisal SNR and temporal stability for symbol's category
selection.
*/
func (field *CategorySurpriseField) ScoreState(
	symbol string,
	selected CategoryType,
) (CategorySurpriseScore, error) {
	if field == nil {
		return CategorySurpriseScore{}, errnie.Error(errors.New("perspectives: CategorySurpriseField nil receiver"))
	}

	if symbol == "" {
		return CategorySurpriseScore{}, errnie.Error(errors.New("perspectives: CategorySurpriseField empty symbol"))
	}

	if selected == CategoryTypeNone {
		return CategorySurpriseScore{}, errnie.Error(fmt.Errorf("perspectives: CategorySurpriseField invalid category for %q", symbol))
	}

	field.mu.Lock()
	tracker, ok := field.trackers[symbol]

	if !ok {
		var err error
		tracker, err = NewCategorySurpriseTracker(field.categories, field.alpha)

		if err != nil {
			field.mu.Unlock()
			return CategorySurpriseScore{}, err
		}

		field.trackers[symbol] = tracker
	}

	field.mu.Unlock()

	return tracker.ScoreState(selected)
}

/*
AssignCategorySurpriseSNR scores temporal surprise from the selected category and
writes the result onto measurement.SNR.
*/
func AssignCategorySurpriseSNR(
	measurement *Measurement,
	field *CategorySurpriseField,
	selected CategoryType,
) error {
	if measurement == nil {
		return errnie.Error(errors.New("perspectives: AssignCategorySurpriseSNR nil measurement"))
	}

	if err := validateUnitMargin("confidence", measurement.Confidence); err != nil {
		return err
	}

	score, err := field.ScoreState(measurement.Symbol, selected)

	if err != nil {
		return err
	}

	measurement.SNR = score.SNR
	measurement.Confidence = score.StabilizeConfidence(measurement.Confidence)

	return nil
}
