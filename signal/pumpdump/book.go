package pumpdump

import (
	"io"
	"strconv"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Book struct {
	clock *structure.ClockRing[*datura.Artifact]
	algo  io.ReadWriteCloser
}

func NewBook() *Book {
	book := &Book{
		clock: structure.NewClockRing[*datura.Artifact](1, 1, 1),
	}

	book.algo = nomagique.Number(
		algorithm.NewBookflowSample(datura.Acquire("pumpdump", datura.APPJSON)),
		equation.NewBookflow(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": equation.BookflowInputKeys,
		})),
		probability.NewClassifier(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"loadedScore",
				"spoofScore",
				"thinScore",
				"neutralScore",
			},
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategoryLoadedImbalance)),
				float64(logic.CategoryIndex(logic.CategorySpoofTrap)),
				float64(logic.CategoryIndex(logic.CategoryBookThinning)),
				float64(logic.CategoryIndex(logic.CategoryDenseNeutrality)),
			},
		})),
	)

	return book
}

func (book *Book) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if err := nomagique.RoundTripArtifact(frame, book.algo); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	return completeBookMeasurement(frame)
}

func completeBookMeasurement(frame *datura.Artifact) *datura.Artifact {
	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		return frame
	}

	loaded := logic.CategoryIndex(logic.CategoryLoadedImbalance)
	spoof := logic.CategoryIndex(logic.CategorySpoofTrap)
	thin := logic.CategoryIndex(logic.CategoryBookThinning)
	neutral := logic.CategoryIndex(logic.CategoryDenseNeutrality)
	baseline := 0.25

	frame.MergeOutputs(map[string]any{
		"loadedScore":         datura.Peek[float64](frame, "output", "loadedScore"),
		"spoofScore":          datura.Peek[float64](frame, "output", "spoofScore"),
		"thinScore":           datura.Peek[float64](frame, "output", "thinScore"),
		"neutralScore":        datura.Peek[float64](frame, "output", "neutralScore"),
		"probabilities":       []float64{baseline, baseline, baseline, baseline},
		"category":            float64(neutral),
		"confidence":          baseline,
		"confidence_baseline": baseline,
		"distribution": map[string]float64{
			strconv.Itoa(loaded):  baseline,
			strconv.Itoa(spoof):   baseline,
			strconv.Itoa(thin):    baseline,
			strconv.Itoa(neutral): baseline,
		},
		"entry_baseline": baseline,
		"exit_baseline":  baseline,
		"strength":       datura.Peek[float64](frame, "output", "strength"),
		"value":          float64(neutral),
	})
	frame.Poke("output", "root")
	frame.Poke([]string{
		"loadedScore",
		"spoofScore",
		"thinScore",
		"neutralScore",
		"probabilities",
		"category",
		"confidence",
		"confidence_baseline",
		"distribution",
		"entry_baseline",
		"exit_baseline",
		"strength",
		"value",
	}, "inputs")

	return frame
}
