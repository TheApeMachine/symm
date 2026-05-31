package economics

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
)

func TestDeskResolveForward(t *testing.T) {
	convey.Convey("Given a tracked entry", t, func() {
		originalWindow := config.System.ExecutionForwardWindow
		config.System.ExecutionForwardWindow = time.Millisecond
		t.Cleanup(func() { config.System.ExecutionForwardWindow = originalWindow })

		desk := NewDesk()
		openedAt := time.Now().Add(-2 * time.Millisecond)
		desk.RecordEntry(Label{
			Event:            "entry",
			Symbol:           "BTC/EUR",
			Playbook:         "trend",
			FillPrice:        100,
			RoundTripCostPct: 0.01,
			At:               openedAt,
		})

		labels := desk.ResolveForward("BTC/EUR", 102, time.Now())

		convey.Convey("It should emit a forward label", func() {
			convey.So(len(labels), convey.ShouldEqual, 1)
			convey.So(labels[0].Event, convey.ShouldEqual, "forward")
			convey.So(labels[0].NetReturn, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestDeskPerformanceSummary(t *testing.T) {
	convey.Convey("Given closed profitable and losing exits", t, func() {
		desk := NewDesk()
		openedAt := time.Now().Add(-1500 * time.Millisecond)
		closedAt := openedAt.Add(500 * time.Millisecond)

		desk.RecordExit(ExitLabel("BTC/EUR", "trend", 100, 104, 0.2, 10, openedAt, closedAt))
		desk.RecordExit(ExitLabel("ETH/EUR", "drive", 100, 98, 0.2, 10, openedAt, closedAt))

		summary := desk.PerformanceSummary()

		convey.Convey("It should expose hold time for profitable exits", func() {
			convey.So(summary.ClosedTrades, convey.ShouldEqual, 2)
			convey.So(summary.ProfitableTrades, convey.ShouldEqual, 1)
			convey.So(summary.LosingTrades, convey.ShouldEqual, 1)
			convey.So(summary.MeanProfitHoldMS, convey.ShouldEqual, 500)
		})
	})
}

func BenchmarkDeskPerformanceSummary(b *testing.B) {
	desk := NewDesk()
	openedAt := time.Now().Add(-time.Second)
	closedAt := openedAt.Add(250 * time.Millisecond)

	for range 64 {
		desk.RecordExit(ExitLabel("BTC/EUR", "trend", 100, 102, 0.2, 10, openedAt, closedAt))
		desk.RecordExit(ExitLabel("ETH/EUR", "drive", 100, 99, 0.2, 10, openedAt, closedAt))
	}

	for b.Loop() {
		_ = desk.PerformanceSummary()
	}
}
