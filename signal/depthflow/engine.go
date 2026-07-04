package depthflow

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
)

type Engine struct {
	sample     *algorithm.BookflowSample
	bookflow   *equation.Bookflow
	classifier *probability.ScoreClassifier
}

func NewEngine() *Engine {
	return &Engine{
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

func (engine *Engine) MeasureBook(frame *datura.Artifact) *datura.Artifact {
	input, ready, err := engine.sample.MeasureBook(algorithm.BookflowBookInput{
		Symbol: datura.Peek[string](frame, "symbol"),
		Bids:   engine.levels(frame, "bids"),
		Asks:   engine.levels(frame, "asks"),
	})

	return engine.measure(frame, input, ready, err)
}

func (engine *Engine) MeasureTrade(frame *datura.Artifact) *datura.Artifact {
	input, ready, err := engine.sample.MeasureTrade(algorithm.BookflowTradeInput{
		Symbol:   datura.Peek[string](frame, "symbol"),
		Price:    datura.Peek[float64](frame, "price"),
		Quantity: datura.Peek[float64](frame, "qty"),
		Side:     datura.Peek[string](frame, "side"),
	})

	return engine.measure(frame, input, ready, err)
}

func (engine *Engine) measure(
	frame *datura.Artifact,
	input equation.BookflowInput,
	ready bool,
	err error,
) *datura.Artifact {
	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	if !ready {
		return nil
	}

	output, err := engine.bookflow.Measure(input)

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	if !output.Ready || output.Strength <= 0 {
		return nil
	}

	frame.MergeOutput("loadedScore", output.LoadedScore)
	frame.MergeOutput("spoofScore", output.SpoofScore)
	frame.MergeOutput("thinScore", output.ThinScore)
	frame.MergeOutput("neutralScore", output.NeutralScore)
	frame.MergeOutput("strength", output.Strength)

	result, err := engine.classifier.Classify(map[string]float64{
		"loadedScore":  output.LoadedScore,
		"spoofScore":   output.SpoofScore,
		"thinScore":    output.ThinScore,
		"neutralScore": output.NeutralScore,
		"strength":     output.Strength,
	})

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	for key, value := range result.Outputs() {
		frame.MergeOutput(key, value)
	}

	return completeMeasurement(frame)
}

func (engine *Engine) levels(frame *datura.Artifact, side string) []algorithm.BookLevel {
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
