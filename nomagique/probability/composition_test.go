package probability_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/probability"
)

func TestEntropyNext(t *testing.T) {
	Convey("Entropy accumulates -p log p and treats zero mass as zero", t, func() {
		for _, test := range []struct {
			values []float64
			want   float64
		}{{[]float64{1}, 0}, {[]float64{.5, .5}, math.Log(2)}, {[]float64{0, 1}, 0}} {
			entropy := probability.NewEntropy()
			var res float64
			for _, v := range test.values {
				res = entropy(v)
			}
			So(res, ShouldAlmostEqual, test.want, 1e-9)
		}
	})
}
