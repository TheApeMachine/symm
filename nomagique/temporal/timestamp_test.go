package temporal_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
)

func TestTimestampNext(t *testing.T) {
	Convey("Timestamp yields Unix nanoseconds for each arrival", t, func() {
		stamp := time.Unix(1700000000, 123)
		at := float64(stamp.UnixNano())
		op := temporal.NewTimestamp()
		adapter := data.NewAdapter(nil, data.NewState(data.NewMap()))
		issued := data.NewOutputMap()
		issued.Values["at"] = at

		for range adapter.Next(data.NewValue(issued)) {
		}

		So(adapter.Error(), ShouldBeNil)

		for range op.Next(data.NewValue(adapter)) {
		}

		So(op.Error(), ShouldBeNil)

		var values data.Map[float64]

		for pointer := range adapter.Next(data.NewValue(data.NewMap("timestamp", "timestamp"))) {
			values = *(*data.Map[float64])(pointer)
		}

		So(adapter.Error(), ShouldBeNil)
		So(values.Values["timestamp"], ShouldEqual, at)
	})
}
