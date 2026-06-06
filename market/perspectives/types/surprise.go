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
	minCategoryProb              = 1e-12
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
	if tracker == nil {
		return 0, errnie.Error(errors.New("perspectives: CategorySurpriseTracker nil receiver"))
	}

	if selected == CategoryTypeNone {
		return 0, errnie.Error(errors.New("perspectives: CategorySurpriseTracker empty category"))
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

	if prob < minCategoryProb {
		prob = minCategoryProb
	}

	surprisal := -math.Log2(prob)

	snrScore, err := tracker.snr.ScoreSurprisal(surprisal)

	if err != nil {
		return 0, err
	}

	for category := range tracker.probs {
		if category == selected {
			tracker.probs[category] = (1-tracker.alpha)*tracker.probs[category] + tracker.alpha
			continue
		}

		tracker.probs[category] = (1 - tracker.alpha) * tracker.probs[category]
	}

	return snrScore, nil
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
	if field == nil {
		return 0, errnie.Error(errors.New("perspectives: CategorySurpriseField nil receiver"))
	}

	if symbol == "" {
		return 0, errnie.Error(errors.New("perspectives: CategorySurpriseField empty symbol"))
	}

	if selected == CategoryTypeNone {
		return 0, errnie.Error(fmt.Errorf("perspectives: CategorySurpriseField invalid category for %q", symbol))
	}

	field.mu.Lock()
	tracker, ok := field.trackers[symbol]

	if !ok {
		var err error
		tracker, err = NewCategorySurpriseTracker(field.categories, field.alpha)

		if err != nil {
			field.mu.Unlock()
			return 0, err
		}

		field.trackers[symbol] = tracker
	}

	field.mu.Unlock()

	return tracker.Score(selected)
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

	snr, err := field.Score(measurement.Symbol, selected)

	if err != nil {
		return err
	}

	measurement.SNR = snr

	return nil
}
