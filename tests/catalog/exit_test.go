package catalog_test

import (
	"testing"

	"github.com/theapemachine/symm/tests/catalog"
)

/*
TestCatalogExitHonestyHoldsUnderPhantomRetreat proves quote-only adverse marks
with retreat evidence do not fire ActionExit — the anti exit-happy gate.
*/
func TestCatalogExitHonestyHoldsUnderPhantomRetreat(t *testing.T) {
	catalog.ProveExit(t, catalog.KindPhantomHold, catalog.Signals)
}

/*
TestCatalogExitHonestyHoldsUnderShallowAdverse proves ~0.8% adverse marks
(audit-trail band) hold when retreat gates the mark.
*/
func TestCatalogExitHonestyHoldsUnderShallowAdverse(t *testing.T) {
	catalog.ProveExit(t, catalog.KindShallowAdverseHold, catalog.Signals)
}

/*
TestCatalogExitHonestyStopsOnSincereDrawdown proves ungated deep breaches still
emit Cause=stop so the hold gate is not a silent freeze forever.
*/
func TestCatalogExitHonestyStopsOnSincereDrawdown(t *testing.T) {
	catalog.ProveExit(t, catalog.KindSincereStop, catalog.Signals)
}

/*
TestCatalogExitHonestyLocksFloorOnCalibratedLift proves marked-up path locks
floor and does not exit while forward forecast remains alive.
*/
func TestCatalogExitHonestyLocksFloorOnCalibratedLift(t *testing.T) {
	catalog.ProveExit(t, catalog.KindCalibratedFloorHold, catalog.Signals)
}

/*
TestCatalogExitHonestyHoldsUngatedAboveStop proves shallow adverse above the
live stop holds without retreat freeze — not only retreat-gated holds.
*/
func TestCatalogExitHonestyHoldsUngatedAboveStop(t *testing.T) {
	catalog.ProveExit(t, catalog.KindUngatedAboveStop, catalog.Signals)
}

/*
TestCatalogExitHonestyMonotoneStopAfterPullback proves an earned floor survives
a pullback that remains above the stop (StopReturn does not loosen).
*/
func TestCatalogExitHonestyMonotoneStopAfterPullback(t *testing.T) {
	catalog.ProveExit(t, catalog.KindMonotonePullback, catalog.Signals)
}

/*
TestCatalogExitHonestyStickyRetreatWithoutReseed proves retreat latched on the
regulator still gates a later CommitStrategy cut that omits retreat measurements.
*/
func TestCatalogExitHonestyStickyRetreatWithoutReseed(t *testing.T) {
	catalog.ProveExit(t, catalog.KindStickyRetreatHold, catalog.Signals)
}

func BenchmarkCatalogExitHonestyShallowHold(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		catalog.ProveExit(b, catalog.KindShallowAdverseHold, catalog.Signals)
	}
}
