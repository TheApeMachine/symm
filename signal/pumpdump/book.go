package pumpdump

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Book struct {
	sample     *algorithm.BookflowSample
	bookflow   *equation.Bookflow
	classifier *probability.ScoreClassifier
}

func NewBook() *Book {
	return &Book{
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

func (book *Book) Measure(row kraken.BookData) (*logic.Measurement, error) {
	input, ready, err := book.sample.MeasureBook(algorithm.BookflowBookInput{
		Symbol: row.Symbol,
		Bids:   book.levels(row.Bids),
		Asks:   book.levels(row.Asks),
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := book.bookflow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !output.Ready || output.Strength <= 0 {
		return nil, nil
	}

	result, err := book.classifier.Classify(map[string]float64{
		"loadedScore":  output.LoadedScore,
		"spoofScore":   output.SpoofScore,
		"thinScore":    output.ThinScore,
		"neutralScore": output.NeutralScore,
		"strength":     output.Strength,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	measurement := logic.NewMeasurement(logic.SourcePumpDump, row.Symbol, row.Timestamp)
	measurement.AddMetric("loadedScore", output.LoadedScore)
	measurement.AddMetric("spoofScore", output.SpoofScore)
	measurement.AddMetric("thinScore", output.ThinScore)
	measurement.AddMetric("neutralScore", output.NeutralScore)
	measurement.AddMetric("strength", output.Strength)

	if err := measurement.ApplyClassifier(
		result.Value,
		result.Confidence,
		result.EntryBaseline,
		result.ExitBaseline,
		result.Strength,
		result.Distribution,
	); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if err := measurement.Ready(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	return measurement, nil
}

func (book *Book) levels(rows []kraken.BookLevel) []algorithm.BookLevel {
	levels := make([]algorithm.BookLevel, 0, len(rows))

	for _, row := range rows {
		levels = append(levels, algorithm.BookLevel{
			Price:    row.Price,
			Quantity: row.Qty,
		})
	}

	return levels
}
