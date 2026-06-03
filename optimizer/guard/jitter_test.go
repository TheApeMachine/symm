package guard

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
)

func TestJitterPerturbBranchValues(t *testing.T) {
	Convey("Given a branch threshold and jitter fraction", t, func() {
		profile := profile.NewProfile(context.Background())
		branches := perspectives.BranchList{
			{
				Category: perspectives.CategoryLaminar,
				Unit:     perspectives.UnitSNR,
				Value:    2,
				ValueSet: true,
			},
		}

		perturbed := perturbBranchValues(branches, 0.1, profile)

		Convey("It should shift the branch value", func() {
			So(perturbed[0].Value, ShouldNotEqual, branches[0].Value)
		})
	})
}

func TestPerturbBranchValueDirect(t *testing.T) {
	Convey("Given direct perturbation", t, func() {
		value := perturbBranchValue(2, 0.5, 0.1)

		Convey("It should add fraction times scale", func() {
			So(value, ShouldAlmostEqual, 2.05, 0.0001)
		})
	})
}
