package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDiagnosticsObserve(t *testing.T) {
	Convey("Score computations roll up into stage clocks", t, func() {
		diagnostics := &Diagnostics{}

		Convey("A module accumulates count, total, last, and max", func() {
			diagnostics.applyModule("category", 100*time.Millisecond)
			diagnostics.applyModule("category", 300*time.Millisecond)

			clock := diagnostics.module("category")
			So(clock.count.Load(), ShouldEqual, 2)
			So(clock.totalNs.Load(), ShouldEqual, 400_000_000)
			So(clock.lastNs.Load(), ShouldEqual, 300_000_000)
			So(clock.maxNs.Load(), ShouldEqual, 300_000_000)
			So(clock.lastAtNs.Load(), ShouldBeGreaterThan, 0)
		})

		Convey("An empty name is ignored", func() {
			diagnostics.applyModule("", 10*time.Millisecond)
			So(diagnostics.module("").count.Load(), ShouldEqual, 0)
		})
	})

	Convey("Hops record the wait between two systems", t, func() {
		diagnostics := &Diagnostics{}

		diagnostics.applyHop("category", "causal", 5*time.Millisecond)
		diagnostics.applyHop("category", "causal", 15*time.Millisecond)

		clock := diagnostics.clocks.hop("category", "causal")
		So(clock.count.Load(), ShouldEqual, 2)
		So(clock.totalNs.Load(), ShouldEqual, 20_000_000)
		So(clock.lastNs.Load(), ShouldEqual, 15_000_000)
	})

	Convey("Errors are retained newest-first as subsystem hints", t, func() {
		diagnostics := &Diagnostics{}

		diagnostics.ObserveError("planner", "search failed", "strategy/planner.go:120")
		diagnostics.ObserveError("desk", "order rejected", "broker/desk.go:400")

		errors := diagnostics.errorSnapshots()
		So(errors, ShouldHaveLength, 2)
		So(errors[0].Source, ShouldEqual, "desk")
		So(errors[1].Source, ShouldEqual, "planner")
		So(errors[0].Caller, ShouldContainSubstring, "broker/desk.go")
		So(errors[0].AtNs, ShouldBeGreaterThan, 0)

		Convey("Unattributed errors default to the system source", func() {
			diagnostics.ObserveError("", "boom", "x.go:1")
			So(diagnostics.errorSnapshots()[0].Source, ShouldEqual, "system")
		})
	})

	Convey("Snapshots enumerate every wired stage in order", t, func() {
		diagnostics := &Diagnostics{}
		diagnostics.applyModule("crypto", time.Millisecond)
		diagnostics.applyModule("desk", 2*time.Millisecond)

		stages := diagnostics.stageSnapshots()
		So(stages, ShouldHaveLength, len(stageNames()))
		So(stages[0].Name, ShouldEqual, "crypto")
		So(stages[len(stages)-1].Name, ShouldEqual, "desk")
		So(stages[0].Count, ShouldEqual, 1)
	})

	Convey("A Crypto without diagnostics reports an idle flow", t, func() {
		crypto := &Crypto{}

		frame := crypto.Diagnostics()
		So(frame.Status, ShouldEqual, "idle")
		So(frame.Stages, ShouldHaveLength, 0)

		Convey("A nil crypto is safe", func() {
			var nilCrypto *Crypto
			So(nilCrypto.Diagnostics().Status, ShouldEqual, "idle")
			So(nilCrypto.Diagnostics().Stages, ShouldHaveLength, 0)
		})
	})

	Convey("The measurement pass distinguishes idle from blocked", t, func() {
		diagnostics := &Diagnostics{}
		now := time.Now()

		Convey("An engine that never ran a pass reports gated idle", func() {
			status := diagnostics.passStatus(now)
			So(status.State, ShouldEqual, "idle")
			So(status.LastPassNs, ShouldEqual, 0)
		})

		Convey("A completed pass reports idle with the pass duration", func() {
			diagnostics.ObservePassStart(now.Add(-10 * time.Millisecond))
			diagnostics.ObservePassEnd(now, 10*time.Millisecond)

			status := diagnostics.passStatus(now)
			So(status.State, ShouldEqual, "idle")
			So(status.LastPassNs, ShouldEqual, 10_000_000)
			So(status.SinceLastNs, ShouldEqual, 0)
		})

		Convey("A pass in flight but under the threshold reports running", func() {
			diagnostics.ObservePassStart(now.Add(-500 * time.Millisecond))

			status := diagnostics.passStatus(now)
			So(status.State, ShouldEqual, "running")
			So(status.InFlightNs, ShouldEqual, 500_000_000)
		})

		Convey("A pass over the threshold reports blocked", func() {
			start := time.Now().Add(-blockedPassThreshold - time.Second)
			diagnostics.ObservePassStart(start)

			status := diagnostics.passStatus(time.Now())
			So(status.State, ShouldEqual, "blocked")
			So(status.InFlightNs, ShouldBeGreaterThan, int64(blockedPassThreshold))
		})
	})
}
