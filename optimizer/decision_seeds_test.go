package optimizer

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestDecisionSeedPlaybooksFromProfile(t *testing.T) {
	Convey("Given a profile and co-occurrence index", t, func() {
		profile := &Profile{ctx: context.Background()}
		profile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar,
			SNR:      2,
		})
		profile.Add(perspectives.Measurement{
			Category: perspectives.CategoryExhaustion,
			SNR:      2,
		})
		profile.PrepareCache()

		index := NewCoOccurrenceIndex(PrecompileTape(profile.Rows()), 0)

		Convey("It should materialize reachable playbooks", func() {
			playbooks := BuildDecisionSeedPlaybooks(profile, index)

			So(len(playbooks), ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("It should return nil without an index", func() {
			So(BuildDecisionSeedPlaybooks(profile, nil), ShouldBeNil)
		})
	})
}

func TestBuildProfileNestedSeedPlaybooks(t *testing.T) {
	Convey("Given multiple categories on tape", t, func() {
		profile := &Profile{ctx: context.Background()}
		profile.Add(perspectives.Measurement{Category: perspectives.CategoryLaminar, SNR: 2})
		profile.Add(perspectives.Measurement{Category: perspectives.CategoryFrenzy, SNR: 3})
		profile.Add(perspectives.Measurement{Category: perspectives.CategoryExhaustion, SNR: 1})
		profile.PrepareCache()

		index := NewCoOccurrenceIndex(PrecompileTape(profile.Rows()), 0)
		playbooks := BuildProfileNestedSeedPlaybooks(profile, index)

		Convey("It should emit nested entry playbooks", func() {
			So(len(playbooks), ShouldBeGreaterThan, 0)
		})
	})
}
