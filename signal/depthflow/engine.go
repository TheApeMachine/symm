package depthflow

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
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

func (engine *Engine) MeasureBook(row kraken.BookData) (*logic.Measurement, error) {
	input, ready, err := engine.sample.MeasureBook(algorithm.BookflowBookInput{
		Symbol: row.Symbol,
		Bids:   engine.levels(row.Bids),
		Asks:   engine.levels(row.Asks),
	})

	return engine.measure(row.Symbol, row.Timestamp, input, ready, err)
}

func (engine *Engine) MeasureTrade(row kraken.TradeData) (*logic.Measurement, error) {
	input, ready, err := engine.sample.MeasureTrade(algorithm.BookflowTradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price,
		Quantity: row.Qty,
		Side:     row.Side,
	})

	return engine.measure(row.Symbol, row.Timestamp, input, ready, err)
}

func (engine *Engine) measure(
	symbol string,
	at time.Time,
	input equation.BookflowInput,
	ready bool,
	err error,
) (*logic.Measurement, error) {
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := engine.bookflow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !output.Ready || output.Strength <= 0 {
		return nil, nil
	}

	result, err := engine.classifier.Classify(map[string]float64{
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

	measurement := logic.NewMeasurement(logic.SourceDepthFlow, symbol, at)
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

func (engine *Engine) levels(rows []kraken.BookLevel) []algorithm.BookLevel {
	levels := make([]algorithm.BookLevel, 0, len(rows))
	for _, row := range rows {
		levels = append(levels, algorithm.BookLevel{
			Price:    row.Price,
			Quantity: row.Qty,
		})
	}

	return levels
}
