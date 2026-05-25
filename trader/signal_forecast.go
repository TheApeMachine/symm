package trader

import (
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

const (
	forecastRejectNoConfidence      = "no_confidence"
	forecastRejectMissingSymbol     = "missing_symbol"
	forecastRejectMissingReturnBook = "missing_return_model"
	forecastRejectMissingQuote      = "missing_quote"
	forecastRejectNoRunway          = "no_runway"
	forecastRejectUncalibrated      = "uncalibrated_return"
)

/*
SignalForecast is the trader-owned profit forecast for one signal reading.
Entry time is implicit at record; exit time is now plus Runway.
*/
type SignalForecast struct {
	ExpectedReturn  float64
	Runway          time.Duration
	CalibrationOnly bool
}

/*
BuildSignalForecast derives expected return from settled forward returns when
enough calibration samples exist; otherwise no forecast is emitted.
*/
func BuildSignalForecast(
	measurement engine.Measurement,
	quote QuoteReader,
	symbol string,
	returnModel *ReturnModel,
) (SignalForecast, bool) {
	forecast, reason := BuildSignalForecastReason(
		measurement, quote, symbol, returnModel,
	)

	return forecast, reason == ""
}

/*
BuildSignalForecastReason derives expected return and reports why a reading cannot
become an executable candidate.
*/
func BuildSignalForecastReason(
	measurement engine.Measurement,
	quote QuoteReader,
	symbol string,
	returnModel *ReturnModel,
) (SignalForecast, string) {
	if measurement.Confidence <= 0 {
		return SignalForecast{}, forecastRejectNoConfidence
	}

	if symbol == "" {
		return SignalForecast{}, forecastRejectMissingSymbol
	}

	if returnModel == nil {
		return SignalForecast{}, forecastRejectMissingReturnBook
	}

	if quote == nil {
		return SignalForecast{}, forecastRejectMissingQuote
	}

	if _, _, _, _, ok := quote.Quote(symbol); !ok {
		return SignalForecast{}, forecastRejectMissingQuote
	}

	runway := forecastRunway(measurement)

	if runway <= 0 {
		return SignalForecast{}, forecastRejectNoRunway
	}

	gross, ok := returnModel.Predict(
		measurement.Source,
		measurement.Regime,
		measurement.Confidence,
	)

	if !ok || gross <= 0 {
		return SignalForecast{}, forecastRejectUncalibrated
	}

	return SignalForecast{
		ExpectedReturn: gross,
		Runway:         runway,
	}, ""
}

func spreadBPSFromQuote(last, bid, ask float64) float64 {
	if last <= 0 || bid <= 0 || ask <= 0 || ask <= bid {
		return 0
	}

	return (ask - bid) / last * 10000
}

func forecastRunway(measurement engine.Measurement) time.Duration {
	switch measurement.Regime {
	case "flow", "depth":
		return config.System.FlowHoldBeforeExit
	case "pump", "momentum", "dump":
		return config.System.ScalpHoldBeforeExit
	case "basis", "cross", "sentiment", "causal":
		return config.System.MinHoldBeforeRotate
	default:
		return config.System.MinHoldBeforeRotate
	}
}
