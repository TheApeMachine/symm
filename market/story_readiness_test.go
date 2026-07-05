package market

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestStoryActiveOrigins(t *testing.T) {
	Convey("Given a story with measured and unready signal measurements", t, func() {
		at := time.Now().UTC()
		story := NewStory(context.Background())

		err := story.Update([]*logic.Measurement{
			{
				Source: logic.SourceCVD,
				Symbol: "BTC/USD",
				At:     at,
				Distribution: map[logic.CategoryType]float64{
					logic.CategoryAggressiveDrive: 1,
				},
				Confidence:    0.8,
				Strength:      0.8,
				EntryBaseline: 0.5,
				ExitBaseline:  0.5,
				Status:        "measured",
			},
			{
				Source: logic.SourceResonance,
				Symbol: "BTC/USD",
				At:     at,
				Distribution: map[logic.CategoryType]float64{
					logic.CategoryLaminar: 1,
				},
				Confidence:    0.7,
				Strength:      0.7,
				EntryBaseline: 0.5,
				ExitBaseline:  0.5,
				Status:        "ambiguous",
			},
		})

		Convey("When active origins are requested", func() {
			origins := story.ActiveOrigins("BTC/USD", at, time.Minute)

			Convey("Then Story.ActiveOrigins only counts measured origins", func() {
				So(err, ShouldBeNil)
				So(origins, ShouldHaveLength, 1)
				So(origins[string(logic.SourceCVD)], ShouldNotBeNil)
				So(origins[string(logic.SourceResonance)], ShouldBeNil)
			})
		})
	})
}
