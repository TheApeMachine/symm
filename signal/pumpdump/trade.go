package pumpdump

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Trade struct {
	sample     *algorithm.TradeFlowSample
	flow       *equation.Flow
	classifier *probability.ScoreClassifier
}

func NewTrade() *Trade {
	return &Trade{
		sample: algorithm.NewTradeFlowSample(),
		flow:   equation.NewFlow(),
		classifier: probability.NewScoreClassifier(
			[]string{"absorption", "drive", "balance", "starvation"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryHiddenAbsorption)),
				float64(logic.CategoryIndex(logic.CategoryAggressiveDrive)),
				float64(logic.CategoryIndex(logic.CategoryStochasticBalance)),
				float64(logic.CategoryIndex(logic.CategoryVolumeStarvation)),
			},
		),
	}
}

func (trade *Trade) Measure(row kraken.TradeData) (*logic.Measurement, error) {
	input, ready, err := trade.sample.Measure(algorithm.TradeFlowInput{
		Symbol:   row.Symbol,
		Price:    row.Price,
		Quantity: row.Qty,
		Side:     row.Side,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := trade.flow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if output.Value <= 0 {
		return nil, nil
	}

	result, err := trade.classifier.Classify(map[string]float64{
		"absorption": output.Absorption,
		"drive":      output.Drive,
		"balance":    output.Balance,
		"starvation": output.Starvation,
		"strength":   output.Value,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	measurement := logic.NewMeasurement(logic.SourcePumpDump, row.Symbol, row.Timestamp)
	measurement.AddMetric("absorption", output.Absorption)
	measurement.AddMetric("drive", output.Drive)
	measurement.AddMetric("balance", output.Balance)
	measurement.AddMetric("starvation", output.Starvation)
	measurement.AddMetric("strength", output.Value)

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
