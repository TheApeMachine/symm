package probability_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestEntropyNext(t *testing.T) {
	Convey("Entropy accumulates -p log p and treats zero mass as zero", t, func() {
		for _, test := range []struct {
			values []float64
			want   float64
		}{{[]float64{1}, 0}, {[]float64{.5, .5}, math.Log(2)}, {[]float64{0, 1}, 0}} {
			out := tests.CollectSeq[float64](probability.NewEntropy().Next(transport.NewValues(test.values...).Next(nil)))
			So(out[len(out)-1], ShouldAlmostEqual, test.want)
		}
	})
}
