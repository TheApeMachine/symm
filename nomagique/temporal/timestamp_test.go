package temporal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestTimestampNext(t *testing.T) {
	Convey("Timestamp yields Unix nanoseconds for each arrival", t, func() {
		stamp := time.Unix(1700000000, 123)
		op := NewTimestamp()
		out := tests.CollectSeq(op.Next(transport.Values(stamp)))

		So(out, ShouldResemble, []int64{stamp.UnixNano()})
		So(op.Error(), ShouldBeNil)
	})
}
