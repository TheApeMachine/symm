package pumpdump

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Book struct {
	clock      *structure.ClockRing[*datura.Artifact]
	sample     *algorithm.BookflowSample
	bookflow   *equation.Bookflow
	classifier *probability.ScoreClassifier
}

func NewBook() *Book {
	return &Book{
		clock:    structure.NewClockRing[*datura.Artifact](1, 1, 1),
		sample:   algorithm.NewBookflowSample(),
		bookflow: equation.NewBookflow(),
		classifier: probability.NewScoreClassifier(
			[]string{"loadedScore", "spoofScore", "thinScore", "neutralScore"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryLoadedImbalance)),
				float64(logic.CategoryIndex(logic.CategorySpoofTrap)),
				float64(logic.CategoryIndex(logic.CategoryBookThinning)),
				float64(logic.CategoryIndex(logic.CategoryDenseNeutrality)),
			},
		),
	}
}

func (book *Book) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	input, ready, err := book.sample.MeasureBook(algorithm.BookflowBookInput{
		Symbol: datura.Peek[string](frame, "symbol"),
		Bids:   book.levels(frame, "bids"),
		Asks:   book.levels(frame, "asks"),
	})

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	if !ready {
		return nil
	}

	output, err := book.bookflow.Measure(input)

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	if !output.Ready || output.Strength <= 0 {
		return nil
	}

	frame.MergeOutput("loadedScore", output.LoadedScore)
	frame.MergeOutput("spoofScore", output.SpoofScore)
	frame.MergeOutput("thinScore", output.ThinScore)
	frame.MergeOutput("neutralScore", output.NeutralScore)
	frame.MergeOutput("strength", output.Strength)
	book.classify(frame, output)

	return completeBookMeasurement(frame)
}

func (book *Book) levels(frame *datura.Artifact, side string) []algorithm.BookLevel {
	levels := make([]algorithm.BookLevel, 0)

	for index := 0; ; index++ {
		price := datura.Peek[float64](frame, side, index, "price")

		if price <= 0 {
			return levels
		}

		levels = append(levels, algorithm.BookLevel{
			Price:    price,
			Quantity: datura.Peek[float64](frame, side, index, "qty"),
		})
	}
}

func (book *Book) classify(frame *datura.Artifact, output equation.BookflowOutput) {
	result, err := book.classifier.Classify(map[string]float64{
		"loadedScore":  output.LoadedScore,
		"spoofScore":   output.SpoofScore,
		"thinScore":    output.ThinScore,
		"neutralScore": output.NeutralScore,
		"strength":     output.Strength,
	})

	if err != nil {
		frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
		return
	}

	for key, value := range result.Outputs() {
		frame.MergeOutput(key, value)
	}
}

func completeBookMeasurement(frame *datura.Artifact) *datura.Artifact {
	evidence := datura.Peek[float64](frame, "output", "loadedScore") +
		datura.Peek[float64](frame, "output", "spoofScore") +
		datura.Peek[float64](frame, "output", "thinScore") +
		datura.Peek[float64](frame, "output", "neutralScore")

	if evidence <= 0 {
		return nil
	}

	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		frame.Poke("output", "root")
		frame.Poke([]string{
			"probabilities",
			"category",
			"confidence",
			"confidence_baseline",
			"distribution",
			"entry_baseline",
			"exit_baseline",
			"strength",
			"loadedScore",
			"spoofScore",
			"thinScore",
			"neutralScore",
		}, "inputs")
		return frame
	}

	return nil
}
