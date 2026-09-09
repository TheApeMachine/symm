package agent

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

func TestAgentContext(t *testing.T) {
	Convey("The agent retains precursor order within the measured producer horizon", t, func() {
		member, _ := newAgentFixture(t)
		at := time.Unix(100, 0)
		first := []uint64{11, 12}
		member.Context("symbol", at, at, first)
		first[0] = 99
		context := member.Context("symbol", at.Add(time.Second), at, []uint64{13})
		So(context, ShouldResemble, []uint64{1<<63 | 2, 11, 12, 1<<63 | 2, 13})
		So(member.Context("symbol", at.Add(2*time.Second), at, []uint64{13}), ShouldResemble, context)
		So(member.Context("symbol", at.Add(3*time.Second), at.Add(time.Second), []uint64{14}), ShouldResemble, []uint64{1<<63 | 2, 13, 1<<63 | 2, 14})
		So(member.Context("other", at, at, []uint64{15}), ShouldResemble, []uint64{1<<63 | 2, 15})
	})
}
