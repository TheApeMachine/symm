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
*/
type Corpus struct {
	*core.PrimitiveError
	maxSize  int
	historyX []float64
	historyY []float64
	input    data.Map[string]
	output   data.Map[float64]
}

func NewCorpus(maxSize int) *Corpus {
	output := data.NewOutputMap()
	output.Values["similarity"] = 0
	output.Values["size"] = 0

	op := &Corpus{
		PrimitiveError: core.NewPrimitiveError(),
		maxSize:        maxSize,
		input: data.NewMap(
			"x", "x",
			"y", "y",
		),
		output: output,
	}

	if maxSize <= 0 {
		op.Error(core.ErrDomain)
	}

	return op
}

func (op *Corpus) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			var values data.Map[float64]

			for pointer := range adapter.Next(data.NewValue(op.input)) {
				values = *(*data.Map[float64])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			coordinateX, xOK := values.Values["x"]
			coordinateY, yOK := values.Values["y"]

			if !xOK || !yOK {
				op.Error(core.ErrNotHeld)
				return
			}

			bestSimilarity := 0.0
			probeNorm := math.Hypot(coordinateX, coordinateY)

			for index := range op.historyX {
				histX := op.historyX[index]
				histY := op.historyY[index]
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

			op.output.Values["similarity"] = bestSimilarity
			op.output.Values["size"] = float64(len(op.historyX))

			for range adapter.Next(data.NewValue(op.output)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
