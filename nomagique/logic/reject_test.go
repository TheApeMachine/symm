package logic

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestRejectNext(t *testing.T) {
	Convey("Reject consumes a run and records the configured reason", t, func() {
		reason := errors.New("refused")
		op := NewReject(reason)
		out := tests.CollectSeq[float64](op.Next(tests.SliceToSeq([]float64{1, 2, 3})))

		So(len(out), ShouldEqual, 0)
		So(errors.Is(op.Error(), reason), ShouldBeTrue)
		So(errors.Is(op.Error(), core.ErrDomain), ShouldBeFalse)
	})
}
