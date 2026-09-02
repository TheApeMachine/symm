package vector

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

var inputSymbols [types.MaxSamples]types.Symbol

func init() {
	for index := range types.MaxSamples {
		inputSymbols[index] = types.MustIntern(fmt.Sprintf("vector/input/%d", index))
	}
}

/*
InputSymbol returns the feature-vector input slot at index.
*/
func InputSymbol(index int) (types.Symbol, bool) {
	if index < 0 || index >= types.MaxSamples {
		return 0, false
	}

	return inputSymbols[index], true
}

/* MustInputSymbol returns the feature-vector input slot or panics. */
func MustInputSymbol(index int) types.Symbol {
	symbol, found := InputSymbol(index)
	if !found {
		panic(fmt.Sprintf("vector: feature index %d is outside capacity", index))
	}
	return symbol
}

/*
FeatureExtractor copies the declared feature vector into generic sample slots.
Until every input is present, SampleReady remains zero and no partial vector is presented.
*/
func FeatureExtractor(input *types.Frame) {
	countValue, found := input.Get(types.SampleCount)
	if !found || countValue <= 0 || float64(int(countValue)) != countValue ||
		int(countValue) > types.MaxSamples {
		input.Err = fmt.Errorf("vector: feature extractor requires a valid sample count")
		return
	}

	count := int(countValue)

	for index := range count {
		input.Delete(types.MustSampleSymbol(index))
	}

	input.Put(types.SampleReady, 0)

	for index := range count {
		value, present := input.Get(inputSymbols[index])
		if !present {
			return
		}
		input.Put(types.MustSampleSymbol(index), value)
	}

	input.Put(types.SampleReady, 1)
}

/*
Extract stages a set of named symbols directly into generic sample slots.
*/
func Extract(symbols ...types.Symbol) types.Primitive {
	count := len(symbols)
	targetSymbols := append([]types.Symbol(nil), symbols...)

	return func(input *types.Frame) {
		if count == 0 || count > types.MaxSamples {
			input.Put(types.SampleReady, 0)
			return
		}

		for index := range count {
			input.Delete(types.MustSampleSymbol(index))
		}
		input.Put(types.SampleReady, 0)

		for index, symbol := range targetSymbols {
			value, present := input.Get(symbol)
			if !present {
				return
			}
			input.Put(types.MustSampleSymbol(index), value)
		}

		input.Put(types.SampleCount, float64(count))
		input.Put(types.SampleReady, 1)
	}
}

/*
ExtractGroups aggregates grouped feature symbols into sample/0..K-1 class scores.
*/
func ExtractGroups(groups ...[]types.Symbol) types.Primitive {
	numClasses := len(groups)
	targetGroups := make([][]types.Symbol, numClasses)
	for i, g := range groups {
		targetGroups[i] = append([]types.Symbol(nil), g...)
	}

	return func(input *types.Frame) {
		if numClasses == 0 || numClasses > types.MaxSamples {
			input.Put(types.SampleReady, 0)
			return
		}

		for index := range numClasses {
			input.Delete(types.MustSampleSymbol(index))
		}
		input.Put(types.SampleReady, 0)

		for _, group := range targetGroups {
			if len(group) == 0 {
				return
			}
			for _, symbol := range group {
				if !input.Has(symbol) {
					return
				}
			}
		}

		for classIdx, group := range targetGroups {
			score := 0.0
			for _, symbol := range group {
				score += input.MustGet(symbol)
			}
			score /= float64(len(group))
			input.Put(types.MustSampleSymbol(classIdx), score)
		}

		input.Put(types.SampleCount, float64(numClasses))
		input.Put(types.SampleReady, 1)
	}
}
