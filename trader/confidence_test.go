package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfidenceAveragesObserve(t *testing.T) {
	Convey("Given a first confidence observation", t, func() {
		averages := newConfidenceAverages()
		value := averages.Observe("hawkes", 0.42)

		Convey("It should seed the live value and snapshot", func() {
			So(value, ShouldAlmostEqual, 0.42)
			So(averages.Snapshot()["hawkes"], ShouldAlmostEqual, 0.42)
		})
	})
}

func BenchmarkConfidenceAveragesObserve(b *testing.B) {
	averages := newConfidenceAverages()

	b.ReportAllocs()

	for b.Loop() {
		_ = averages.Observe("hawkes", 0.42)
	}
}
