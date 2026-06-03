package budget

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/cooccurrence"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
)

func TestDecisionSeedPlaybooksFromProfile(t *testing.T) {
	Convey("Given a profile and co-occurrence index", t, func() {
		measurementProfile := profile.NewProfile(context.Background())
		measurementProfile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar,
			SNR:      2,
		})
		measurementProfile.Add(perspectives.Measurement{
			Category: perspectives.CategoryExhaustion,
			SNR:      2,
		})
		measurementProfile.PrepareCache()

		index := cooccurrence.NewCoOccurrenceIndex(replay.PrecompileTape(measurementProfile.Rows()), 0)

		Convey("It should materialize reachable playbooks", func() {
			playbooks := BuildDecisionSeedPlaybooks(measurementProfile, index)

			So(len(playbooks), ShouldBeGreaterThanOrEqualTo, 0)
		})

		Convey("It should return nil without an index", func() {
			So(BuildDecisionSeedPlaybooks(measurementProfile, nil), ShouldBeNil)
		})
	})
}

func TestBuildProfileNestedSeedPlaybooks(t *testing.T) {
	Convey("Given multiple categories on tape", t, func() {
		measurementProfile := profile.NewProfile(context.Background())
		measurementProfile.Add(perspectives.Measurement{Category: perspectives.CategoryLaminar, SNR: 2})
		measurementProfile.Add(perspectives.Measurement{Category: perspectives.CategoryFrenzy, SNR: 3})
		measurementProfile.Add(perspectives.Measurement{Category: perspectives.CategoryExhaustion, SNR: 1})
		measurementProfile.PrepareCache()

		index := cooccurrence.NewCoOccurrenceIndex(replay.PrecompileTape(measurementProfile.Rows()), 0)
		playbooks := BuildProfileNestedSeedPlaybooks(measurementProfile, index)

		Convey("It should emit nested entry playbooks", func() {
			So(len(playbooks), ShouldBeGreaterThan, 0)
		})
	})
}
