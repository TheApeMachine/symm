package pumpdump

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	sample     *flow.Sample
	bookflow   *equation.Bookflow
	classifier *probability.ScoreClassifier
}

func NewBook() *Book {
	return &Book{
		sample:   flow.NewSample(),
		bookflow: equation.NewBookflow(),
		classifier: probability.NewScoreClassifier(
			[]string{"loadedScore", "spoofScore", "thinScore", "neutralScore"},
			[]float64{
				float64(types.CategoryIndex(types.CategoryLoadedImbalance)),
				float64(types.CategoryIndex(types.CategorySpoofTrap)),
				float64(types.CategoryIndex(types.CategoryBookThinning)),
				float64(types.CategoryIndex(types.CategoryDenseNeutrality)),
			},
		),
	}
}

func (book *Book) Measure(row kraken.BookData) ([]*types.Measurement, error) {
	bids, err := book.levels(row.Bids, row.PriceIncrement)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	asks, err := book.levels(row.Asks, row.PriceIncrement)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	input, ready, err := book.sample.MeasureBook(flow.BookInput{
		Symbol:   row.Symbol,
		TickSize: row.PriceIncrement.Float64(),
		Bids:     bids,
		Asks:     asks,
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

	categories := []types.CategoryType{
		types.LoadedImbalance,
		types.SpoofTrap,
		types.BookThinning,
		types.DenseNeutrality,
	}
	strengths := []float64{
		output.LoadedScore,
		output.SpoofScore,
		output.ThinScore,
		output.NeutralScore,
	}
	categoryRows := make([]types.Category, 0, len(categories))

	for index, category := range categories {
		confidence := 0.0

		if index < len(result.Probabilities) {
			confidence = result.Probabilities[index]
		}

		categoryRows = append(categoryRows, types.Category{
			Type:       category,
			Confidence: confidence,
			Strength:   strengths[index],
		})
	}

	measurement := &types.Measurement{
		Source:        types.SourcePumpDump,
		Stream:        "book",
		Symbol:        row.Symbol,
		At:            row.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"loadedScore":  output.LoadedScore,
			"spoofScore":   output.SpoofScore,
			"thinScore":    output.ThinScore,
			"neutralScore": output.NeutralScore,
			"strength":     output.Strength,
			"value":        output.Value,
			"category":     output.Category,
		},
	}

	return []*types.Measurement{measurement}, nil
}

func (book *Book) levels(
	rows []kraken.BookLevel,
	increment decimal.Decimal,
) ([]flow.BookLevel, error) {
	levels := make([]flow.BookLevel, 0, len(rows))

	for _, row := range rows {
		tick, err := kraken.PriceTick(row.Price, increment)

		if err != nil {
			return nil, err
		}

		levels = append(levels, flow.BookLevel{
			Price:    row.Price.Float64(),
			Ticks:    tick,
			Quantity: row.Qty,
		})
	}

	return levels, nil
}
