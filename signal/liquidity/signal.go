package liquidity

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
Liquidity is the Scarcity perspective, identifying opportunities in thin markets
by ranking a symbol's volume against the broader market.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	classifier *probability.ScoreClassifier
}

func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		classifier: probability.NewScoreClassifier(
			[]string{"scarcityScore", "medianScore", "depthScore"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryExtremeScarcity)),
				float64(logic.CategoryIndex(logic.CategoryMedianDepth)),
				float64(logic.CategoryIndex(logic.CategoryRobustLiquidity)),
			},
		),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(
	input market.Input,
	crossSection *market.CrossSection,
) ([]*logic.Measurement, error) {
	if crossSection == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "liquidity: cross-section required", nil,
		))
	}

	if input.Role != "ticker" {
		return nil, nil
	}

	measurements := make([]*logic.Measurement, 0, len(input.Ticker))
	for _, ticker := range input.Ticker {
		measurement, err := signal.measure(ticker, crossSection)

		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func (signal *Signal) measure(
	ticker kraken.TickerData,
	crossSection *market.CrossSection,
) (*logic.Measurement, error) {
	peers := crossSection.Volumes()

	if len(peers) < 2 {
		return nil, nil
	}

	median, ok := statistic.MedianOf(peers)

	if !ok || median <= 0 {
		return nil, nil
	}

	relative := ticker.Volume / median
	scarcity := math.Max(0, 1-relative)
	depth := math.Max(0, relative-1)
	balance := 1 / (1 + math.Abs(relative-1))
	strength := max(scarcity, max(balance, depth))

	result, err := signal.classifier.Classify(map[string]float64{
		"scarcityScore": scarcity,
		"medianScore":   balance,
		"depthScore":    depth,
		"strength":      strength,
	})

	if err != nil {
		return nil, err
	}

	measurement := logic.NewMeasurement(logic.SourceLiquidity, ticker.Symbol, ticker.Timestamp)
	measurement.AddMetric("relativeVolume", relative)
	measurement.AddMetric("scarcityScore", scarcity)
	measurement.AddMetric("medianScore", balance)
	measurement.AddMetric("depthScore", depth)
	measurement.AddMetric("strength", strength)

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

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.err
}
