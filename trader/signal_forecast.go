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
BuildSignalForecast derives expected return and hold horizon from a signal
reading and the live quote. Signals never populate these fields themselves.
*/
func BuildSignalForecast(
	measurement engine.Measurement,
	quote QuoteReader,
	symbol string,
) (SignalForecast, bool) {
	if measurement.Confidence <= 0 || symbol == "" {
		return SignalForecast{}, false
	}

	last, bid, ask, _, ok := quote.Quote(symbol)

	if !ok || last <= 0 {
		return SignalForecast{}, false
	}

	spreadBPS := spreadBPSFromQuote(last, bid, ask)

	if spreadBPS <= 0 {
		return SignalForecast{}, false
	}

	runway := forecastRunway(measurement)

	if runway <= 0 {
		return SignalForecast{}, false
	}

	expectedReturn := measurement.Confidence * (spreadBPS / 10000)

	if expectedReturn <= 0 {
		return SignalForecast{}, false
	}

	return SignalForecast{
		ExpectedReturn: expectedReturn,
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
	case "flow":
		return config.System.FlowHoldBeforeExit
	case "pump", "momentum", "dump":
		return config.System.ScalpHoldBeforeExit
	case "causal":
		return config.System.MinHoldBeforeRotate
	default:
		return config.System.MinHoldBeforeRotate
	}
}
