package trader

import (
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
)

/*
SignalForecast is the trader-owned profit forecast for one signal reading.
Entry time is implicit at record; exit time is now plus Runway.
*/
type SignalForecast struct {
	ExpectedReturn float64
	Runway         time.Duration
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
	if measurement.Confidence <= 0 || symbol == "" || returnModel == nil {
		return SignalForecast{}, false
	}

	if _, _, _, _, ok := quote.Quote(symbol); !ok {
		return SignalForecast{}, false
	}

	runway := forecastRunway(measurement)

	if runway <= 0 {
		return SignalForecast{}, false
	}

	gross, ok := returnModel.Predict(
		measurement.Source,
		measurement.Regime,
		measurement.Confidence,
	)

	if !ok || gross <= 0 {
		return SignalForecast{}, false
	}

	return SignalForecast{
		ExpectedReturn: gross,
		Runway:         runway,
	}, true
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
