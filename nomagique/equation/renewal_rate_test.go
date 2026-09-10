package equation_test

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewRenewalRate(t *testing.T) {
	Convey("A non-positive target is a domain failure", t, func() {
		op := equation.NewRenewalRate(0)
		out := tests.CollectSeq(op.Next(transport.Values(equation.RenewalInput{
			Increment: 2, Sample: 100, At: 0,
		})))
		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), core.ErrDomain), ShouldBeTrue)
	})

	Convey("Quantity accumulates until the target and elapsed time are met", t, func() {
		op := equation.NewRenewalRate(4)
		out := tests.CollectSeq(op.Next(transport.Values(
			equation.RenewalInput{Increment: 2, Sample: 100, At: 0},
			equation.RenewalInput{Increment: 2, Sample: 100, At: 1e9},
		)))
		So(out[0].Closed, ShouldBeFalse)
		So(out[1].Closed, ShouldBeTrue)
		So(out[1].Rate, ShouldEqual, 4)
	})
}
