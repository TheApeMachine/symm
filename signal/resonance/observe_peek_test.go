package resonance

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSignalObserveBooks(t *testing.T) {
	Convey("Given typed resonance book rows", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		scope := "FLOW/EUR"
		seedBook(t, signal, scope, 1, 0.001, startAt(0))

		Convey("When the book window is inspected", func() {
			spread := signal.book.Spread(scope)
			window, ok := signal.book.Window(scope)
			features, featureOK := signal.featureContext(scope)

			Convey("Then spread is retained for resonance features", func() {
				So(spread, ShouldBeGreaterThan, 0)
				So(ok, ShouldBeTrue)
				So(window.Spreads, ShouldNotBeEmpty)
				So(window.Spreads[len(window.Spreads)-1], ShouldBeGreaterThan, 0)
				So(featureOK, ShouldBeFalse)
				So(features.spread, ShouldEqual, 0)
			})
		})
	})
}

func TestSignalObserveTickers(t *testing.T) {
	Convey("Given old and current typed ticker rows", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		seedTicker(t, signal, "OLD/USD", 1, 1, 0.1, startAt(0))
		seedTicker(t, signal, "NOW/USD", 42, 1, 0.1, startAt(1))

		Convey("When ticker snapshots are inspected", func() {
			oldSnapshot := signal.ticker.Snapshot("OLD/USD")
			nowSnapshot := signal.ticker.Snapshot("NOW/USD")

			Convey("Then each symbol retains its own current row", func() {
				So(oldSnapshot.Last, ShouldEqual, 1)
				So(nowSnapshot.Last, ShouldEqual, 42)
				So(nowSnapshot.Observed.Equal(startAt(1)), ShouldBeTrue)
			})
		})
	})
}
