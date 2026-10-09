package hindsight

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestStoreTee_Push(t *testing.T) {
	Convey("Given a StoreTee", t, func() {
		tee := NewStoreTee(context.Background(), "test")
		row := data.NewMeasurement(1, "BTC/USD", "detector", 1, 1)
		row.At, row.From = time.Unix(1_700_000_000, 0).UTC(), time.Unix(1_700_000_000, 0).UTC()

		Convey("A push before READY is dropped and counted", func() {
			So(tee.Status(), ShouldNotEqual, runtime.READY)
			tee.Push(row)
			So(tee.Dropped(), ShouldEqual, 1)
			So(tee.Pending(), ShouldEqual, 0)
		})

		Convey("A push while READY is queued and taken", func() {
			tee.Transition(runtime.READY)
			tee.Push(row)
			So(tee.Dropped(), ShouldEqual, 0)
			So(tee.Take(), ShouldResemble, []*data.Measurement{row})
			So(tee.Pending(), ShouldEqual, 0)
		})
	})
}
