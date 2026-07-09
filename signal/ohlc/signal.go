package ohlc

import (
	"context"
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal evaluates macro-volatility candles over fixed time horizons.
It analyzes the relationship between the candle's body size, total range,
and distance from the Volume-Weighted Average Price (VWAP).
*/
type Signal[T any] struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	classifier *probability.ScoreClassifier
}

func NewSignal[T any](ctx context.Context) *Signal[T] {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal[T]{
		ctx:    ctx,
		cancel: cancel,
		classifier: probability.NewScoreClassifier(
			[]string{"drive", "balance", "reversal", "starvation"},
			[]float64{
				float64(types.CategoryIndex(types.CategoryAggressiveDrive)),
				float64(types.CategoryIndex(types.CategoryStochasticBalance)),
				float64(types.CategoryIndex(types.CategoryActiveReversal)),
				float64(types.CategoryIndex(types.CategoryVolumeStarvation)),
			},
		),
	}
}

func (signal *Signal[T]) IngestRoles() []string {
	return []string{"ohlc"}
}

func (signal *Signal[T]) Categories() []types.CategoryType {
	return []types.CategoryType{
		types.AggressiveDrive,
		types.StochasticBalance,
		types.ActiveReversal,
		types.VolumeStarvation,
	}
}

func (signal *Signal[T]) Measure(
	input T,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	row, ok := any(input).(kraken.OHLCData)

	if !ok {
		return nil, nil
	}

	high := row.High
	low := row.Low
	closePrice := row.Close
	open := row.Open

	if high <= low || closePrice <= 0 || open <= 0 {
		return nil, nil
	}

	rangeSpan := high - low

	// Close location ratio within total High-Low range [0.0, 1.0]
	closeLocation := (closePrice - low) / rangeSpan

	// Candle body size relative to total wick-to-wick range
	bodySize := math.Abs(closePrice-open) / rangeSpan

	driveScore := 0.0
	balanceScore := 0.0
	reversalScore := 0.0
	starvationScore := 0.0

	// 1. Aggressive Drive: Large body closing near extreme highs or lows
	if closeLocation > 0.85 || closeLocation < 0.15 {
		driveScore = bodySize
	}

	// 2. Stochastic Balance: Close settles near the center
	if closeLocation >= 0.4 && closeLocation <= 0.6 {
		balanceScore = 1.0 - bodySize
	}

	// 3. Active Reversal: Long-wick patterns (Pin-Bars/Dojis)
	if bodySize < 0.30 && (closeLocation > 0.70 || closeLocation < 0.30) {
		reversalScore = 1.0 - (bodySize * 2.5)
	}

	// 4. Volume Starvation: Low execution frequency proxy
	if row.Trades < 10 {
		starvationScore = 1.0
	}

	strength := max(driveScore, max(balanceScore, max(reversalScore, starvationScore)))

	if strength <= 0 {
		strength = 0.01
	}

	result, err := signal.classifier.Classify(map[string]float64{
		"drive":      driveScore,
		"balance":    balanceScore,
		"reversal":   reversalScore,
		"starvation": starvationScore,
		"strength":   strength,
	})

	if err != nil {
		return nil, errnie.Error(err)
	}

	categories := []types.CategoryType{
		types.AggressiveDrive,
		types.StochasticBalance,
		types.ActiveReversal,
		types.VolumeStarvation,
	}
	strengths := []float64{
		driveScore,
		balanceScore,
		reversalScore,
		starvationScore,
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
		Source:        types.SourceOHLC,
		Stream:        "ohlc",
		Symbol:        row.Symbol,
		At:            row.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"close_location": closeLocation,
			"body_size":      bodySize,
			"vwap_distance":  row.Close - row.Vwap,
			"drive":          driveScore,
			"balance":        balanceScore,
			"reversal":       reversalScore,
			"starvation":     starvationScore,
			"strength":       strength,
		},
	}

	return []*types.Measurement{measurement}, nil
}

func (signal *Signal[T]) Error() error {
	return signal.err
}

func (signal *Signal[T]) Close() error {
	signal.cancel()

	return nil
}