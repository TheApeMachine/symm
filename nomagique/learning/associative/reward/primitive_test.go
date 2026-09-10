package reward_test

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPrimitiveNext(t *testing.T) {
	Convey("The ledger measures marks in producer order and refuses regressions", t, func() {
		node := reward.New()
		start := time.Unix(1700000000, 0)
		marks := []reward.Mark{
			{At: start, Version: 1, Value: 100},
			{At: start.Add(time.Second), Version: 2, Value: 110},
			{At: start.Add(3 * time.Second), Version: 3, Value: 90},
			{At: start.Add(3 * time.Second), Version: 3, Value: 90},
			{At: start.Add(3 * time.Second), Version: 4, Value: 95},
			{At: start.Add(10 * time.Second), Version: 5, Value: 130},
		}
		output := tests.CollectSeq(node.Next(transport.Values(marks...)))
		So(node.Error(), ShouldBeNil)
		So(len(output), ShouldEqual, len(marks))

		for index, want := range []float64{0, 10, -20, -20, 5, 35} {
			So(output[index].Reward, ShouldEqual, want)
		}

		So(output[2].Differential, ShouldEqual, -40)
		So(output[4].Differential, ShouldEqual, 5)
		So(output[5].Rate, ShouldEqual, 3)
		So(output[5].TotalReward, ShouldEqual, 30)
		So(output[5].Transitions, ShouldEqual, 4)

		coarse := tests.CollectSeq(reward.New().Next(transport.Values(marks[0], marks[5])))
		So(coarse[1].Rate, ShouldEqual, 3)

		bad := tests.CollectSeq(node.Next(transport.Values(marks[1])))
		So(node.Error(), ShouldNotBeNil)
		So(len(bad), ShouldEqual, 0)
	})
}
