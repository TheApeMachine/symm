package geometry

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Corpus retains a bounded history of coordinates and scores similarity against history.
Yields similarity and size.
*/
type Corpus struct {
	*core.PrimitiveError
	maxSize  int
	historyX []float64
	historyY []float64
}

func NewCorpus(maxSize int) *Corpus {
	op := &Corpus{
		PrimitiveError: core.NewPrimitiveError(),
		maxSize:        maxSize,
	}

	if maxSize <= 0 {
		op.Error(core.ErrDomain)
	}

	return op
}

func (op *Corpus) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var values [2]float64
		index := 0

		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			if index < 2 {
				values[index] = *(*float64)(arriving)
				index++
			}
		}

		if index < 2 {
			op.Error(core.ErrShape)
			return
		}

		coordinateX := values[0]
		coordinateY := values[1]

		bestSimilarity := 0.0
		probeNorm := math.Hypot(coordinateX, coordinateY)

		for historyIndex := range op.historyX {
			histX := op.historyX[historyIndex]
			histY := op.historyY[historyIndex]
			dot := coordinateX*histX + coordinateY*histY
			histNorm := math.Hypot(histX, histY)

			if probeNorm > 0 && histNorm > 0 {
				similarity := dot / (probeNorm * histNorm)

				if similarity > bestSimilarity {
					bestSimilarity = similarity
				}
			}
		}

		op.historyX = append(op.historyX, coordinateX)
		op.historyY = append(op.historyY, coordinateY)

		if len(op.historyX) > op.maxSize {
			op.historyX = append([]float64(nil), op.historyX[len(op.historyX)-op.maxSize:]...)
			op.historyY = append([]float64(nil), op.historyY[len(op.historyY)-op.maxSize:]...)
		}

		size := float64(len(op.historyX))

		for value := range data.NewValue(bestSimilarity, size).Next(nil) {
			if !yield(value) {
				return
			}
		}
	}
}
