package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestEvidenceCompositions(t *testing.T) {
	Convey("ValidPair validates non-zero predicted and actual", t, func() {
		op := equation.NewValidPair()
		in1 := equation.ValidPairInput{Predicted: 1.5, Actual: 2.0}
		in2 := equation.ValidPairInput{Predicted: 0.0, Actual: 2.0}
		out := tests.CollectSeq[bool](op.Next(transport.NewValues(in1, in2).Next(nil)))
		So(op.Error(), ShouldBeNil)
		So(out, ShouldResemble, []bool{true, false})
	})

	Convey("EvidenceAuthority weights SNR and maturity", t, func() {
		op := equation.NewEvidenceAuthority()
		in := equation.EvidenceAuthorityInput{
			Estimated:  true,
			SNRDefined: true,
			SNR:        3.0,
			Maturity:   0.8,
			Zero:       0.0,
			Unknown:    0.5,
		}
		out := tests.CollectSeq[float64](op.Next(transport.NewValues(in).Next(nil)))
		So(op.Error(), ShouldBeNil)
		// factor = 3 / (1 + 3) = 0.75. value = 0.8 * 0.75 = 0.6
		So(out[0], ShouldAlmostEqual, 0.6, 1e-9)
	})

	Convey("EvidenceShare normalizes and selects element at index", t, func() {
		op := equation.NewEvidenceShare(1)
		v := []float64{1.0, 2.0, 1.0} // sum = 4, index 1 is 2.0/4.0 = 0.5
		out := tests.CollectSeq[float64](op.Next(transport.NewValues(v).Next(nil)))
		So(op.Error(), ShouldBeNil)
		So(out[0], ShouldAlmostEqual, 0.5, 1e-9)
	})
}
