package types

import (
	"slices"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
uiRoute holds the dashboard's active client route (surface) so the UI Tee
can drop high-frequency metrics not needed by the currently viewed page.
*/
var uiRoute atomic.Value

func init() {
	uiRoute.Store("dashboard")
}

/*
SetRoute records the active dashboard page / surface route 
(e.g. "fluid", "dashboard", "learning").
*/
func SetRoute(route string) {
	route = strings.TrimPrefix(strings.TrimSpace(route), "/")
	if route == "" {
		route = "dashboard"
	}

	uiRoute.Store(route)
}

/*
Route returns the current active dashboard page / surface route.
*/
func Route() string {
	value := uiRoute.Load()

	if value == nil {
		return "dashboard"
	}

	route, ok := value.(string)

	if !ok || route == "" {
		return "dashboard"
	}

	return route
}

/*
AllowsRoute reports whether a measurement should go on the dashboard websocket
for the current page. Raw venue feeds stay off the wire. Focus still limits
which symbol is published so the UI is not flooded.
*/
func Filters(measurement *data.Measurement[float64]) bool {
	if measurement == nil {
		return false
	}

	switch Route() {
	case "dashboard":
		if !isFocus(measurement) {
			return false
		}

		return isSignal(measurement) || isLogic(measurement, "resonance", "category", "cognition")
	case "learning":
		return isStrategy(measurement, "training")
	case "xray":
		if !isFocus(measurement) {
			return false
		}

		return isSignal(measurement, "hawkes") || isLogic(measurement, "resonance")
	case "fluid":
		if _, ok := measurement.Result.(*ManifoldState); ok {
			return true
		}

		return isLogic(measurement, "manifold")
	default:
		return false
	}
}

func isFocus(measurement *data.Measurement[float64]) bool {
	if Focus() == "" {
		return true
	}

	return measurement.Label == Focus()
}

func isSignal(measurement *data.Measurement[float64], signals ...string) bool {
	if len(signals) == 0 {
		switch kernelSource(measurement.Source) {
		case "websocket", "correlation", "cvd", "depthflow", "derivatives", "hawkes", "leadlag", "liquidity", "morphology", "pumpdump", "sentiment", "toxicity":
			return true
		default:
			return false
		}
	}
	if kernelSource(measurement.Source) == "websocket" {
		return true
	}
	return slices.Contains(signals, kernelSource(measurement.Source))
}

func isLogic(measurement *data.Measurement[float64], solverNames ...string) bool {
	if kernelSource(measurement.Source) == "websocket" {
		return true
	}
	return slices.Contains(solverNames, kernelSource(measurement.Source))
}

func isStrategy(measurement *data.Measurement[float64], strategies ...string) bool {
	if kernelSource(measurement.Source) == "websocket" {
		return true
	}
	return slices.Contains(strategies, kernelSource(measurement.Source))
}

func kernelSource(source string) string {
	if before, _, ok := strings.Cut(source, ":"); ok {
		return before
	}

	return source
}
