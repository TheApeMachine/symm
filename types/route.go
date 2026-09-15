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
SetRoute records the active dashboard page / surface route (e.g. "fluid", "dashboard", "learning").
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
AllowsRoute reports whether a measurement from the given source should be
transmitted over the dashboard WebSocket while on the current route.
*/
func AllowsRoute(measurement *data.Measurement[float64]) bool {
	switch Route() {
	case "dashboard":
		return isSignalAndFocus(measurement, SignalSourceStrings...) || isLogicAndFocus(measurement, "resonance")
	case "learning":
		return isStrategyAndFocus(measurement, "training")
	case "fluid":
		return isLogic(measurement, "manifold")
	case "journal", "hindsight", "workbench", "pipeline":
		return false
	default:
		return true
	}
}

func isSignalAndFocus(measurement *data.Measurement[float64], signals ...string) bool {
	return isSignal(measurement, signals...) && measurement.Label == Focus()
}

func isLogicAndFocus(measurement *data.Measurement[float64], solverNames ...string) bool {
	return isLogic(measurement, solverNames...) && measurement.Label == Focus()
}

func isStrategyAndFocus(measurement *data.Measurement[float64], strategies ...string) bool {
	return isStrategy(measurement, strategies...) && measurement.Label == Focus()
}

func isSignal(measurement *data.Measurement[float64], signals ...string) bool {
	return slices.Contains(
		signals, measurement.Source,
	)
}

func isLogic(measurement *data.Measurement[float64], solverNames ...string) bool {
	return slices.Contains(
		solverNames, measurement.Source,
	)
}

func isStrategy(measurement *data.Measurement[float64], strategies ...string) bool {
	return slices.Contains(
		strategies, measurement.Source,
	)
}
