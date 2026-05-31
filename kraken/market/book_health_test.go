package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type captureBookHealthSink struct {
	events []BookHealthEvent
}

func (sink *captureBookHealthSink) BookHealth(event BookHealthEvent) {
	sink.events = append(sink.events, event)
}

func TestRecordBookDivergence(t *testing.T) {
	Convey("Given a fresh book health tracker", t, func() {
		ResetBookHealth()

		RecordBookDivergence("fluid", "BTC/EUR")
		summary := BookIntegritySummary()

		Convey("It should count the symbol as diverged", func() {
			So(summary.DivergedSymbols, ShouldEqual, 1)
			So(summary.DivergenceEvents, ShouldEqual, 1)
			So(summary.Diverged, ShouldContain, "BTC/EUR")
		})

		Convey("It should not double-count the same symbol", func() {
			RecordBookDivergence("fluid", "BTC/EUR")
			summary := BookIntegritySummary()

			So(summary.DivergedSymbols, ShouldEqual, 1)
			So(summary.DivergenceEvents, ShouldEqual, 1)
		})
	})
}

func TestRecordBookRecoveryRequiresMatchingSignal(t *testing.T) {
	Convey("Given one signal has a diverged book", t, func() {
		ResetBookHealth()
		sink := &captureBookHealthSink{}
		SetBookHealthSink(sink)
		t.Cleanup(func() {
			SetBookHealthSink(nil)
			ResetBookHealth()
		})

		RecordBookDivergence("depthflow", "ETH/EUR")
		RecordBookRecovery("fluid", "ETH/EUR")
		summary := BookIntegritySummary()

		Convey("It should keep the original signal marked diverged", func() {
			So(summary.DivergedSymbols, ShouldEqual, 1)
			So(summary.Diverged, ShouldContain, "ETH/EUR")
			So(sink.events, ShouldHaveLength, 1)
		})

		Convey("It should recover only when the same signal realigns", func() {
			RecordBookRecovery("depthflow", "ETH/EUR")
			summary := BookIntegritySummary()

			So(summary.DivergedSymbols, ShouldEqual, 0)
			So(sink.events, ShouldHaveLength, 2)
			So(sink.events[1].Recovered, ShouldBeTrue)
			So(sink.events[1].Signal, ShouldEqual, "depthflow")
		})
	})
}

func BenchmarkRecordBookDivergenceRecovery(b *testing.B) {
	SetBookHealthSink(nil)
	b.ReportAllocs()

	for b.Loop() {
		ResetBookHealth()
		RecordBookDivergence("depthflow", "BTC/EUR")
		RecordBookRecovery("depthflow", "BTC/EUR")
	}
}

func TestRecordBookRecovery(t *testing.T) {
	Convey("Given a diverged symbol", t, func() {
		ResetBookHealth()
		RecordBookDivergence("depthflow", "ETH/EUR")
		RecordBookRecovery("depthflow", "ETH/EUR")
		summary := BookIntegritySummary()

		Convey("It should clear the divergence", func() {
			So(summary.DivergedSymbols, ShouldEqual, 0)
		})
	})
}
