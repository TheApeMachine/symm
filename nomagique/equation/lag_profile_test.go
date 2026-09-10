package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

type stubEstimator struct{}

func (stubEstimator) Estimate(left, right *equation.LogReturns, lag int64) (equation.LagEstimate, error) {
	return equation.LagEstimate{Correlation: float64(lag)}, nil
}

func TestNewLagProfile(t *testing.T) {
	Convey("Lag profile preserves each discrete position", t, func() {
		left := []equation.Price{{At: 0, Value: 1}, {At: 1e9, Value: 2}, {At: 2e9, Value: 1.5}}
		right := []equation.Price{{At: 1e9, Value: 1}, {At: 2e9, Value: 1.5}, {At: 3e9, Value: 2}}
		op := equation.NewLagProfile(stubEstimator{}, 1e9, 2)
		profile := tests.CollectSeq(op.Next(transport.Values(equation.LagProfileInput{Left: left, Right: right})))
		So(op.Error(), ShouldBeNil)
		So(len(profile), ShouldEqual, 5)

		for index, candidate := range profile {
			So(candidate.Index, ShouldEqual, index)
			So(candidate.LagIndex, ShouldEqual, float64(index)-2)
		}
	})
}
