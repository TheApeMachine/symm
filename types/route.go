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
AllowsRoute reports whether a measurement should go on the dashboard websocket
for the current page. Raw venue feeds stay off the wire. Focus still limits
which symbol is published so the UI is not flooded.
*/
func AllowsRoute(measurement *data.Measurement[float64]) bool {
	if measurement == nil || measurement.Label == "" {
		return false
	}

	// The resonance channel carries the complete predictive result. Publishing
	// its scalar summary here would overwrite that result in the same UI store.
	if isLogic(measurement, "resonance") {
		return false
	}

	switch Route() {
	case "dashboard":
		return isSignal(measurement, SignalSourceStrings...) &&
			Allows(measurement.Label)
	case "xray":
		return isSignal(measurement, "hawkes") && Allows(measurement.Label)
	case "learning":
		return isStrategy(measurement, "training") && Allows(measurement.Label)
	case "fluid":
		return isLogic(measurement, "manifold")
	case "journal", "hindsight", "workbench", "pipeline":
		return false
	default:
		return (isSignal(measurement, SignalSourceStrings...) || isLogic(measurement, LogicSourceStrings...)) &&
			Allows(measurement.Label)
	}
}

/*
AllowsWebRTC selects structured results before the Tee retains them. X-Ray needs
all symbols for its universe scatter; other predictive views need only focus.
The fluid state is global and belongs only to the fluid surface.
*/
func AllowsWebRTC(source, label string) bool {
	switch kernelSource(source) {
	case "manifold":
		return Route() == "fluid"
	case "resonance":
		return label != "" && (Route() == "xray" ||
			(Route() == "dashboard" && Allows(label)))
	}

	return false
}

/*
RouteDropReason names why AllowsRoute rejected a measurement. Empty means it
would be published. Used to prove where the live websocket goes silent.
*/
func RouteDropReason(measurement *data.Measurement[float64]) string {
	if measurement == nil {
		return "nil"
	}

	if measurement.Label == "" {
		return "empty-label"
	}

	if AllowsRoute(measurement) {
		return ""
	}

	if isLogic(measurement, "resonance") {
		return "webrtc"
	}

	if Route() == "dashboard" && !isSignal(measurement, SignalSourceStrings...) {
		return "source"
	}

	if !Allows(measurement.Label) {
		return "focus"
	}

	return "route"
}

func isSignal(measurement *data.Measurement[float64], signals ...string) bool {
	return slices.Contains(signals, kernelSource(measurement.Source))
}

func isLogic(measurement *data.Measurement[float64], solverNames ...string) bool {
	return slices.Contains(solverNames, kernelSource(measurement.Source))
}

func isStrategy(measurement *data.Measurement[float64], strategies ...string) bool {
	return slices.Contains(strategies, kernelSource(measurement.Source))
}

func kernelSource(source string) string {
	if index := strings.IndexByte(source, ':'); index >= 0 {
		return source[:index]
	}

	return source
}
