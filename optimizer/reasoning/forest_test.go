package reasoning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestMergeSeedForests(t *testing.T) {
	Convey("Given two category seeds", t, func() {
		vocab := Vocabulary{
			Categories: []types.CategoryType{types.CategoryType("alpha"), types.CategoryType("beta")},
			Entries:    []reasoning.ActionType{reasoning.ActionMarket},
			Thresholds: []float64{1},
			Offsets:    []float64{0.02},
		}
		forests := Seeds(vocab)

		Convey("MergeSeedForests should carry both entry roots and one management root", func() {
			merged := MergeSeedForests(forests[:2])

			So(ForestStrategyCount(merged), ShouldEqual, 2)
			So(len(merged), ShouldEqual, 3)
		})
	})
}
