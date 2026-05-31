package economics

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestRejectRegretResolveForward(t *testing.T) {
	convey.Convey("Given a tracked gate reject", t, func() {
		originalWindow := config.System.ExecutionForwardWindow
		config.System.ExecutionForwardWindow = time.Millisecond
		t.Cleanup(func() { config.System.ExecutionForwardWindow = originalWindow })

		regret := NewRejectRegret()
		rejectedAt := time.Now().Add(-2 * time.Millisecond)
		regret.Track("BTC/EUR", "trend", "systemic_slump_wait", 100, 0.01, 1.0, rejectedAt)

		regret.ResolveForward("BTC/EUR", 102, time.Now())
		summary := regret.Summary()

		convey.Convey("It should count a profitable missed entry", func() {
			convey.So(summary.GateRejectsTracked, convey.ShouldEqual, 1)
			convey.So(summary.GateRejectsResolved, convey.ShouldEqual, 1)
			convey.So(summary.MissedProfitable, convey.ShouldEqual, 1)
			convey.So(summary.MissedForwardEUR, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestRejectRegretDedupe(t *testing.T) {
	convey.Convey("Given duplicate rejects inside the dedupe window", t, func() {
		regret := NewRejectRegret()
		regret.SetDedupeWindow(time.Minute)
		now := time.Now()

		regret.Track("BTC/EUR", "trend", "systemic_slump_wait", 100, 0.01, 1.0, now)
		regret.Track("BTC/EUR", "trend", "systemic_slump_wait", 100, 0.01, 1.0, now.Add(time.Second))

		convey.Convey("It should track only one sample", func() {
			convey.So(regret.Summary().GateRejectsTracked, convey.ShouldEqual, 1)
		})
	})
}

func BenchmarkRejectRegretTrack(b *testing.B) {
	regret := NewRejectRegret()
	now := time.Now()

	for b.Loop() {
		regret.Track("BTC/EUR", "trend", "systemic_slump_wait", 100, 0.01, 1.0, now)
	}
}
