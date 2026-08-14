package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbol_Measure(t *testing.T) {
	Convey("Given an identified symbol-local arrival process", t, func() {
		model := newSymbol()
		stream, horizon := burstStream(
			time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
			32,
		)
		fitted, ready := model.measure(stream, horizon)

		Convey("It should retain the fitted epoch for exact intensity projection", func() {
			So(ready, ShouldBeTrue)
			So(fitted.Readiness.ModelUpdated, ShouldBeTrue)

			projected, projectedReady := model.measure(stream, horizon)

			So(projectedReady, ShouldBeTrue)
			So(projected.Readiness.HawkesFit, ShouldBeTrue)
			So(projected.Readiness.ModelUpdated, ShouldBeFalse)
			So(projected.FitObservedAt, ShouldResemble, fitted.FitObservedAt)
		})
	})
}
