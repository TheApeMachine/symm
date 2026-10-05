package cognition

import (
	"bytes"
	"iter"
	"math"
	"math/rand"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type Dreamer struct {
	*core.PrimitiveError
	store core.Primitive
}

func NewDreamer(store core.Primitive) *Dreamer {
	return &Dreamer{
		PrimitiveError: core.NewPrimitiveError(),
		store:          store,
	}
}

// Dream generates a candidate continuation sequence via temperature-guided lookahead.
func (dreamer *Dreamer) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				dreamer.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				dreamer.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				dreamer.Error(err)
				return
			}

			if len(sequence) == 0 {
				maxLength = 64
			}

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				dreamer.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}

	root := op.Root()
	currentSequence := ""

	for hop := 0; hop < maxLength; hop++ {
		searchPrefix := makeSensoryKey([]byte(currentSequence))
		iterator := root.Root().Iterator()
		iterator.SeekPrefix(searchPrefix)

		var candidates []string
		var probabilities []float64

		for keyBytes, valBytes, ok := iterator.Next(); ok; keyBytes, valBytes, ok = iterator.Next() {
			if !bytes.HasPrefix(keyBytes, searchPrefix) {
				break
			}

			seqSuffix := string(keyBytes[len("s/"):])

			if len(seqSuffix) <= len(currentSequence) {
				continue
			}

			weight := decodeWeight(valBytes)
			candidates = append(candidates, seqSuffix)
			probabilities = append(probabilities, math.Max(weight.Probability, 1e-4))
		}

		if len(candidates) == 0 {
			break
		}

		if temperature <= 0 {
			bestIdx := 0
			bestProb := probabilities[0]

			for candIdx := 1; candIdx < len(probabilities); candIdx++ {
				if probabilities[candIdx] > bestProb {
					bestProb = probabilities[candIdx]
					bestIdx = candIdx
				}
			}

			currentSequence = candidates[bestIdx]
			continue
		}

		totalScaled := 0.0
		scaledWeights := make([]float64, len(probabilities))

		for candIdx, probVal := range probabilities {
			scaled := math.Pow(probVal, 1.0/temperature)
			scaledWeights[candIdx] = scaled
			totalScaled += scaled
		}

		sampleVal := rand.Float64() * totalScaled
		runningSum := 0.0
		selectedCandidate := candidates[len(candidates)-1]

		for candIdx, scaled := range scaledWeights {
			runningSum += scaled

			if sampleVal <= runningSum {
				selectedCandidate = candidates[candIdx]
				break
			}
		}

		currentSequence = selectedCandidate
	}

	return currentSequence
}
