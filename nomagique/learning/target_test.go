package learning_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestTargetsNext(t *testing.T) {
	Convey("Each target preserves its source formula", t, func() {
		for _, test := range []struct {
			name string
			node core.Primitive[learning.Observation, float64]
			want func(current, past float64) float64
		}{
			{"delta", learning.NewDeltaTarget(), func(current, past float64) float64 { return current - past }},
			{"identity", learning.NewIdentityTarget(), func(current, past float64) float64 { return current }},
			{"ratio", learning.NewRatioTarget(), func(current, past float64) float64 { return current/past - 1 }},
			{"binary", learning.NewBinaryTarget(), func(current, past float64) float64 {
				if current > past {
					return 1
				}

				return 0
			}},
			{"directional", learning.NewDirectionalTarget(0.5), func(current, past float64) float64 {
				if math.Abs(current-past) > 0.5 {
					return math.Copysign(1, current-past)
				}

				return 0
			}},
		} {
			Convey(test.name, func() {
				for _, pair := range [][2]float64{{2, 1}, {-1, 2}, {0, 2}, {2, 2}, {2.5, 2}, {-2, -3}} {
					got, err := transport.Evaluate(test.node, transport.Values(learning.Observation{
						Current: pair[0], Past: pair[1],
					}))
					So(err, ShouldBeNil)
					So(got, ShouldEqual, test.want(pair[0], pair[1]))
				}
			})
		}
	})
}

func TestTargetInvalidInput(t *testing.T) {
	Convey("Non-finite values and a zero ratio divisor fail explicitly", t, func() {
		for _, poison := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			_, err := transport.Evaluate(learning.NewDeltaTarget(), transport.Values(learning.Observation{
				Current: poison, Past: 1,
			}))
			So(err, ShouldNotBeNil)
		}

		_, err := transport.Evaluate(learning.NewRatioTarget(), transport.Values(learning.Observation{
			Current: 1, Past: 0,
		}))
		So(err, ShouldNotBeNil)
	})
}

func TestTargetConfiguredConnection(t *testing.T) {
	Convey("A live deadband is configuration of the same target", t, func() {
		node := learning.NewDirectionalTarget(0.5)
		got, err := transport.Evaluate(node, transport.Values(learning.Observation{Current: 2, Past: 1}))
		So(err, ShouldBeNil)
		So(got, ShouldEqual, 1)
		node.Deadband = 2
		got, err = transport.Evaluate(node, transport.Values(learning.Observation{Current: 2, Past: 1}))
		So(err, ShouldBeNil)
		So(got, ShouldEqual, 0)
		out := tests.CollectSeq(node.Next(transport.Values(
			learning.Observation{Current: 2, Past: 1},
			learning.Observation{Current: 2, Past: 1},
		)))
		So(len(out), ShouldEqual, 2)
	})
}

func TestDirectionalTargetInvalidConfiguration(t *testing.T) {
	Convey("A negative or non-finite deadband is refused", t, func() {
		for _, band := range []float64{-1, math.NaN(), math.Inf(1)} {
			_, err := transport.Evaluate(learning.NewDirectionalTarget(band), transport.Values(learning.Observation{
				Current: 2, Past: 1,
			}))
			So(err, ShouldNotBeNil)
		}
	})
}
