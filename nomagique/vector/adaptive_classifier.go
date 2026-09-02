package vector

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Group declares one classification target class and its constituent metric keys.
*/
type Group struct {
	Label   string
	Symbols []types.Symbol
}

/*
NewGroup pre-interns string metric keys into types.Symbol at construction time.
*/
func NewGroup(label string, keys ...string) Group {
	symbols := make([]types.Symbol, len(keys))
	for i, k := range keys {
		symbols[i] = types.MustIntern(k)
	}
	return Group{Label: label, Symbols: symbols}
}

type standardizedMetric struct {
	raw          types.Symbol
	value        types.Symbol
	standardized types.Symbol
	standardize  types.Primitive
}

/*
AdaptiveClassifier builds a unified, zero-allocation feature extraction and classification equation:
1. Standardizes all incoming metrics with adaptive.Standardizer.
2. Fails when any declared metric across any group is missing.
3. Averages standardized z-scores per group into class logits (sample/0..K-1).
4. Evaluates Softmax probabilities, Argmax winner, and Shannon Ambiguity.
*/
func AdaptiveClassifier(groups ...Group) types.Primitive {
	numClasses := len(groups)

	seen := make(map[types.Symbol]struct{})
	metrics := make([]standardizedMetric, 0)

	for _, group := range groups {
		for _, symbol := range group.Symbols {
			if _, exists := seen[symbol]; exists {
				continue
			}

			name, _ := types.SymbolName(symbol)
			prefix := "vector/feature/" + name

			metrics = append(metrics, standardizedMetric{
				raw:          symbol,
				value:        types.MustIntern(prefix + "/value"),
				standardized: types.MustIntern(prefix + "/z/value"),
				standardize:  adaptive.Standardizer(prefix),
			})
			seen[symbol] = struct{}{}
		}
	}

	require := RequireGroups(groups...)

	return func(frame *types.Frame) {
		if numClasses == 0 || numClasses > types.MaxSamples {
			frame.Err = errnie.Error(errnie.Err(
				errnie.Validation,
				"adaptive classifier: class count is invalid",
				nil,
			))

			return
		}

		require(frame)

		if frame.Err != nil {
			return
		}

		for _, metric := range metrics {
			frame.Put(metric.value, frame.MustGet(metric.raw))
			metric.standardize(frame)

			if frame.Err != nil {
				return
			}

			frame.Put(metric.raw, frame.MustGet(metric.standardized))
		}

		for classIndex, group := range groups {
			logit := 0.0

			for _, symbol := range group.Symbols {
				logit += frame.MustGet(symbol)
			}

			logit /= float64(len(group.Symbols))
			frame.Put(types.MustSampleSymbol(classIndex), logit)
		}

		frame.Put(types.SampleCount, float64(numClasses))
		frame.Put(types.SampleReady, 1)

		probability.Classifier(frame)
	}
}

/* RequireGroups verifies that every declared metric exists in this Frame. */
func RequireGroups(groups ...Group) types.Primitive {
	return func(frame *types.Frame) {
		frame.Err = ValidateGroups(frame, groups)
	}
}

/* ValidateGroups returns an error unless every declared metric is present. */
func ValidateGroups(frame *types.Frame, groups []Group) error {
	for _, group := range groups {
		if len(group.Symbols) == 0 {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"adaptive classifier: group has no metrics: "+group.Label,
				nil,
			))
		}

		for _, symbol := range group.Symbols {
			if frame.Has(symbol) {
				continue
			}

			name, _ := types.SymbolName(symbol)
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"adaptive classifier: required metric missing: "+name,
				nil,
			))
		}
	}

	return nil
}

/* GroupsComplete reports whether every declared metric is present. */
func GroupsComplete(frame *types.Frame, groups []Group) bool {
	for _, group := range groups {
		for _, symbol := range group.Symbols {
			if !frame.Has(symbol) {
				return false
			}
		}
	}

	return true
}
