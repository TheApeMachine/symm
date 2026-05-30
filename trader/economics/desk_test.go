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
