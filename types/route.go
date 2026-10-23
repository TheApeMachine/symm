package types

import (
	"slices"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/symm/data"
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
func Filters(value *data.Value) bool {
	if value == nil {
		return false
	}

	if kernelSource(value.Origin) == "balance" {
		return true
	}

	switch Route() {
	case "dashboard":
		if isStrategy(value, "training") {
			return true
		}
		return isFocus(value) && (isSignal(value) || isLogic(value, "resonance"))
	case "learning":
		return isStrategy(value, "training")
	case "longtail":
		// The training performance surface reads the strategy's learning
		// stream: the training node, the grammar feed, and the cognition
		// match frames.
		source := kernelSource(value.Origin)
		return source == "training" || source == "sequitur" || source == "cognition"
	case "xray":
		if isSignal(value, "hawkes") {
			return isFocus(value)
		}

		return isLogic(value, "resonance", "manifold", "cognition")
	case "fluid":
		return isLogic(value, "manifold")
	case "diagnostics":
		return isFocus(value)
	default:
		return false
	}
}

/*
Shard partitions measurement sources deterministically across the dual UI tees.
It divides load equally across both queues, encoding workers, and websockets.
*/
func Shard(source string) int {
	clean := kernelSource(source)

	switch clean {
	case
		"balance",
		"correlation",
		"depthflow",
		"hawkes",
		"liquidity",
		"pumpdump",
		"resonance",
		"training",
		"websocket":
		return 0
	case
		"cvd",
		"derivatives",
		"leadlag",
		"morphology",
		"sentiment",
		"toxicity",
		"manifold",
		"cognition":
		return 1
	default:
		var hash uint32 = 2166136261
		for i := 0; i < len(clean); i++ {
			hash ^= uint32(clean[i])
			hash *= 16777619
		}
		return int(hash % 2)
	}
}

func isFocus(value *data.Value) bool {
	if Focus() == "" {
		return true
	}

	return value.Scope == Focus()
}

func isSignal(value *data.Value, signals ...string) bool {
	if len(signals) == 0 {
		switch kernelSource(value.Origin) {
		case "correlation",
			"cvd",
			"depthflow",
			"derivatives",
			"hawkes",
			"leadlag",
			"liquidity",
			"morphology",
			"pumpdump",
			"sentiment",
			"toxicity":
			return true
		default:
			return false
		}
	}

	return slices.Contains(signals, kernelSource(value.Origin))
}

func isLogic(value *data.Value, solverNames ...string) bool {
	if kernelSource(value.Origin) == "websocket" {
		return true
	}
	return slices.Contains(solverNames, kernelSource(value.Origin))
}

func isStrategy(value *data.Value, strategies ...string) bool {
	return slices.Contains(strategies, kernelSource(value.Origin))
}

func kernelSource(source string) string {
	if before, _, ok := strings.Cut(source, ":"); ok {
		return before
	}

	return source
}
