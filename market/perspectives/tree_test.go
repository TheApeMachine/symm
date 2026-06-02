package perspectives

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewTree(t *testing.T) {
	Convey("Given embedded playbook config", t, func() {
		ctx := context.Background()

		tree, err := NewTree(ctx, nil)

		So(err, ShouldBeNil)

		Convey("It should load branches from cfg", func() {
			So(len(tree.Branches()), ShouldBeGreaterThan, 0)
		})

		Convey("It should walk with measurements", func() {
			action := tree.Walk([]Measurement{
				{
					Source:   SourceFluid,
					Category: CategoryLaminar,
					SNR:      2,
				},
			})

			So(tree.Action(), ShouldEqual, action)
		})
	})
}

func TestNewTreeFromBranches(t *testing.T) {
	Convey("Given in-memory branches", t, func() {
		ctx := context.Background()
		branches := BranchList{
			{
				Category:    CategoryLaminar,
				Observation: ObservationNotHolding,
				Unit:        UnitSNR,
				Condition:   ConditionIsGreaterThanOrEqual,
				Value:       1,
				ValueSet:    true,
				Action:      Action{Type: ActionLimit},
			},
		}

		tree, err := NewTreeFromBranches(ctx, branches)

		So(err, ShouldBeNil)
		So(len(tree.Branches()), ShouldEqual, 1)
	})
}

func TestTreeResetWalk(t *testing.T) {
	Convey("Given a walked tree", t, func() {
		ctx := context.Background()
		tree, err := NewTreeFromBranches(ctx, BranchList{
			{
				Category:    CategoryLaminar,
				Observation: ObservationNotHolding,
				Unit:        UnitSNR,
				Condition:   ConditionIsGreaterThanOrEqual,
				Value:       1,
				ValueSet:    true,
				Action:      Action{Type: ActionLimit},
			},
		})

		So(err, ShouldBeNil)
		tree.WalkContext(BranchContext{
			Measurements: []Measurement{{Source: SourceFluid, Category: CategoryLaminar, SNR: 2}},
			Observations: map[ObservationType]float64{ObservationNotHolding: 1},
		})
		So(tree.Action(), ShouldNotBeNil)
		So(tree.WalkAudit(), ShouldNotBeNil)
		So(len(tree.WalkAudit().Steps), ShouldBeGreaterThan, 0)

		tree.ResetWalk()

		Convey("It should clear the current action", func() {
			So(tree.Action(), ShouldBeNil)
		})
	})
}

func BenchmarkTreeWalk(b *testing.B) {
	ctx := context.Background()
	tree, _ := NewTree(ctx, nil)
	rows := []Measurement{{Source: SourceFluid, Category: CategoryLaminar, SNR: 2}}

	for b.Loop() {
		tree.Walk(rows)
	}
}
