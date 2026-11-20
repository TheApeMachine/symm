package types

import (
	"strings"
	"sync/atomic"
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
Route returns the active dashboard surface.
*/
func Route() string {
	value, _ := uiRoute.Load().(string)
	if value == "" {
		return "dashboard"
	}

	return value
}

/*
WantsGrid reports whether the active surface consumes per-tick regions/paths.
Longtail and workbench never paint the grid; other surfaces may, so publishers
still throttle rather than relying on this alone.
*/
func WantsGrid() bool {
	switch Route() {
	case "longtail", "workbench":
		return false
	default:
		return true
	}
}
