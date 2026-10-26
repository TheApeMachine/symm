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
