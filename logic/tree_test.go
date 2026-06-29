package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	balancefixtures "github.com/theapemachine/symm/tests/fixtures/balances"
)

func TestTreeEvaluateSequentialMeasurementStream(testingTB *testing.T) {
	Convey("Given a two-stage playbook branch and a balances fixture", testingTB, func() {
		tree := &Tree{}
		branches := []*Branch{
			{
				ConditionGroup: &ConditionGroup{
					Boolean: BooleanTypeAnd,
					Conditions: []Condition{
						{
							Type: ConditionIsTrue,
							Left: ConditionOperand{
								Type:     SubjectCategory,
								Source:   SourcePumpDump,
								Category: NewCategory(CategoryCoiledCompression),
							},
						},
					},
				},
				Branches: []*Branch{
					{
						ConditionGroup: &ConditionGroup{
							Boolean: BooleanTypeAnd,
							Conditions: []Condition{
								{
									Type: ConditionIsTrue,
									Left: ConditionOperand{
										Type:     SubjectCategory,
										Source:   SourcePumpDump,
										Category: NewCategory(CategoryVerticalIgnition),
									},
								},
							},
						},
						Action: &Action{
							Type:     ActionMarket,
							Side:     SideBuy,
							Quantity: 0.1,
						},
					},
				},
			},
		}

		var holdings *datura.Artifact
		for artifact := range balancefixtures.NewFixture(balancefixtures.SNAPSHOT, 1).Artifacts() {
			holdings = artifact
			break
		}

		first := testMeasurementArtifact(SourcePumpDump, "MATIC/USD", CategoryCoiledCompression, 0.71, 1)
		first.SetTimestamp(time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC).UnixNano())
		second := testMeasurementArtifact(SourcePumpDump, "MATIC/USD", CategoryVerticalIgnition, 0.83, 1)
		second.SetTimestamp(time.Date(2026, 6, 29, 8, 0, 1, 0, time.UTC).UnixNano())

		Convey("When the first stream tick is evaluated", func() {
			actions, err := tree.Evaluate([]*datura.Artifact{first}, holdings, branches)

			Convey("Then it should only arm the sequential parent", func() {
				So(err, ShouldBeNil)
				So(actions, ShouldBeEmpty)
			})
		})

		Convey("When the next stream tick confirms the child branch", func() {
			_, err := tree.Evaluate([]*datura.Artifact{first}, holdings, branches)
			So(err, ShouldBeNil)

			actions, err := tree.Evaluate([]*datura.Artifact{second}, holdings, branches)

			Convey("Then it should emit one buy candidate with measurement evidence", func() {
				So(err, ShouldBeNil)
				So(actions, ShouldHaveLength, 1)
				So(actions[0].Symbol, ShouldEqual, "MATIC/USD")
				So(actions[0].EntryConfidence, ShouldEqual, 0.83)
				So(actions[0].ReasonCategory, ShouldEqual, CategoryVerticalIgnition)
			})
		})
	})
}
