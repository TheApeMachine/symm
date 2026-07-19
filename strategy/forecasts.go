package strategy

import "github.com/theapemachine/symm/types"

/*
selectForecasts picks one forecast per symbol: highest SourceEpoch wins, then
Eligible over ineligible, then first seen. Silent last-write maps are forbidden.
*/
func selectForecasts(rows []types.Forecasts) map[string]types.Forecasts {
	selected := make(map[string]types.Forecasts, len(rows))

	for _, forecast := range rows {
		prior, found := selected[forecast.Symbol]

		if !found {
			selected[forecast.Symbol] = forecast
			continue
		}

		if forecast.SourceEpoch > prior.SourceEpoch {
			selected[forecast.Symbol] = forecast
			continue
		}

		if forecast.SourceEpoch < prior.SourceEpoch {
			continue
		}

		if forecast.Eligible() && !prior.Eligible() {
			selected[forecast.Symbol] = forecast
		}
	}

	return selected
}

/*
selectForecast returns the preferred forecast for symbol, or false when absent.
*/
func selectForecast(rows []types.Forecasts, symbol string) (types.Forecasts, bool) {
	selected := selectForecasts(rows)
	forecast, found := selected[symbol]

	return forecast, found
}
