package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/rest"
)

/*
StartupWarm loads Kraken OHLC history and seeds signal tracks plus calibrators.
*/
func StartupWarm(
	ctx context.Context,
	pairIndex map[string]asset.Pair,
	symbols []string,
	signals []Signal,
) (map[string][]OHLCCandle, error) {
	if !config.System.OHLCEWarmEnabled {
		return nil, nil
	}

	if len(symbols) == 0 {
		return nil, fmt.Errorf("ohlc warm requires symbols")
	}

	candles, err := loadOHLCSnapshots(ctx, pairIndex, symbols)

	if err != nil {
		return nil, err
	}

	if len(candles) == 0 {
		return nil, fmt.Errorf("ohlc warm returned no candles")
	}

	applyOHLCWarm(signals, candles)

	return candles, nil
}

func loadOHLCSnapshots(
	ctx context.Context,
	pairIndex map[string]asset.Pair,
	symbols []string,
) (map[string][]OHLCCandle, error) {
	interval := config.System.OHLCIntervalMinutes

	if interval <= 0 {
		interval = 5
	}

	delay := config.System.OHLCFetchDelay

	if delay <= 0 {
		delay = 200 * time.Millisecond
	}

	targets := ohlcWarmSymbols(symbols)
	candles := make(map[string][]OHLCCandle, len(targets))

	for _, symbol := range targets {
		if ctx.Err() != nil {
			return candles, ctx.Err()
		}

		pair, ok := pairIndex[symbol]

		if !ok {
			continue
		}

		pairName := asset.Symbol(pair)

		bars, err := rest.FetchOHLC(pairName, interval)

		if err != nil {
			errnie.Error(fmt.Errorf("ohlc warm %s: %w", symbol, err))

			time.Sleep(delay)

			continue
		}

		if len(bars) > 0 {
			candles[symbol] = toOHLCCandles(bars)
		}

		time.Sleep(delay)
	}

	return candles, nil
}

func ohlcWarmSymbols(symbols []string) []string {
	maxSymbols := config.System.OHLCMaxSymbols

	if maxSymbols <= 0 || len(symbols) <= maxSymbols {
		return symbols
	}

	return append([]string(nil), symbols[:maxSymbols]...)
}

func toOHLCCandles(bars []rest.Candle) []OHLCCandle {
	candles := make([]OHLCCandle, len(bars))

	for index, bar := range bars {
		candles[index] = OHLCCandle{
			Time:   bar.Time,
			Open:   bar.Open,
			High:   bar.High,
			Low:    bar.Low,
			Close:  bar.Close,
			Volume: bar.Volume,
		}
	}

	return candles
}

func applyOHLCWarm(signals []Signal, candles map[string][]OHLCCandle) {
	runway := time.Duration(config.System.OHLCIntervalMinutes) * time.Minute

	if runway <= 0 {
		runway = 5 * time.Minute
	}

	for _, signal := range signals {
		if signal == nil {
			continue
		}

		if warmer, ok := signal.(OHLCWarmer); ok {
			warmer.WarmFromOHLC(candles)
		}

		for _, feedback := range CalibrationFeedbackFromOHLC(signal.Source(), candles, runway) {
			signal.Feedback(feedback)
		}
	}
}

/*
CalibrationFeedbackFromOHLC synthesizes settled prediction feedback from bar ranges.
*/
func CalibrationFeedbackFromOHLC(
	source string,
	candles map[string][]OHLCCandle,
	runway time.Duration,
) []PredictionFeedback {
	feedback := make([]PredictionFeedback, 0, len(candles)*8)

	for symbol, bars := range candles {
		completed := CompletedCandles(bars)

		if len(completed) < 2 {
			continue
		}

		for index := 0; index < len(completed)-1; index++ {
			bar := completed[index]
			next := completed[index+1]

			if bar.Close <= 0 {
				continue
			}

			predicted := (bar.High - bar.Low) / bar.Close

			if predicted <= 0 {
				continue
			}

			actual := (next.Close - bar.Close) / bar.Close

			feedback = append(feedback, PredictionFeedback{
				Source:          source,
				Symbol:          symbol,
				PredictedReturn: predicted,
				ActualReturn:    actual,
				Runway:          runway,
			})
		}
	}

	return feedback
}
