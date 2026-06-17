package market

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

func measurementArtifact(measurement logic.Measurement) *datura.Artifact {
	payload, _ := json.Marshal(measurement)

	return datura.Acquire(
		"test", datura.Artifact_Type_json,
	).WithRole(
		"measurement",
	).WithScope(
		measurement.Symbol,
	).WithPayload(
		payload,
	)
}

func TestStoryMeasurements(t *testing.T) {
	Convey("Given a story with stored measurements", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)

		story := NewStory(ctx, pool)

		So(story, ShouldNotBeNil)

		fluid := logic.Measurement{
			Source: logic.SourceFluid,
			Symbol: "ETH/USD",
		}
		hawkes := logic.Measurement{
			Source: logic.SourceHawkes,
			Symbol: "ETH/USD",
		}

		So(story.Update(measurementArtifact(fluid)), ShouldBeNil)
		So(story.Update(measurementArtifact(hawkes)), ShouldBeNil)

		Convey("When Measurements is called", func() {
			measurements := story.Measurements()

			Convey("It should return every stored measurement", func() {
				So(len(measurements), ShouldEqual, 2)
			})
		})

		Convey("When DecisionTreeBranches is called", func() {
			branches := story.DecisionTreeBranches()

			Convey("It should expose the embedded playbook", func() {
				So(len(branches), ShouldBeGreaterThan, 0)
			})
		})
	})
}
