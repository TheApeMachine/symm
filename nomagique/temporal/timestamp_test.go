package temporal_test

import (
	"testing"
	"time"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestTimestampNext(t *testing.T) {
	Convey("Timestamp yields Unix nanoseconds for each arrival", t, func() {
		stamp := time.Unix(1700000000, 123)
		op := temporal.NewTimestamp()
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&stamp))
		}
		out := tests.CollectSeq[int64](op.Next(in))

		So(out, ShouldResemble, []int64{stamp.UnixNano()})
		So(op.Error(), ShouldBeNil)
	})
}
