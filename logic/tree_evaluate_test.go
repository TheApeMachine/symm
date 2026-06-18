package logic

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool")

func TestTreeEvaluateExitBranch(testingTB *testing.T) {
	convey.Convey("Given embedded playbook branches", testingTB, func() {
		pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
		tree, decodeErr := NewTree(context.Background(), pool)

		convey.Convey("It should decode without error", func() {
			convey.So(decodeErr, convey.ShouldBeNil)
			convey.So(tree, convey.ShouldNotBeNil)
			convey.So(len(tree.Branches), convey.ShouldBeGreaterThan, 0)
		})

		measurements := []Measurement{
			{
				Source:     SourceExhaustion,
				Symbol:     "SOL/EUR",
				Category:   CategoryMechanicalCollapse,
				Confidence: 0.85,
				Strength:   1,
			},
		}

		holdings := &Balances{
			Inventory: map[string]float64{"SOL/EUR": 1.5},
		}

		results, evaluateErr := tree.Evaluate(measurements, holdings, tree.Branches)

		convey.Convey("It should match a held exit branch", func() {
			convey.So(evaluateErr, convey.ShouldBeNil)
			convey.So(len(results), convey.ShouldBeGreaterThan, 0)

			foundExit := false

			for _, action := range results {
				if action != nil && action.Type == ActionSettlePosition {
					foundExit = true
				}
			}

			convey.So(foundExit, convey.ShouldBeTrue)
		})
	})
}

func TestTreeEvaluateEmptyMeasurements(testingTB *testing.T) {
	convey.Convey("Given no measurements", testingTB, func() {
		pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
		tree, decodeErr := NewTree(context.Background(), pool)

		convey.So(decodeErr, convey.ShouldBeNil)

		results, evaluateErr := tree.Evaluate(nil, &Balances{}, tree.Branches)

		convey.Convey("It should return no actions", func() {
			convey.So(evaluateErr, convey.ShouldBeNil)
			convey.So(results, convey.ShouldBeNil)
		})
	})
}

func TestBranchEvaluateDirect(testingTB *testing.T) {
	convey.Convey("Given a branch with a direct action", testingTB, func() {
		branch := &Branch{
			Action: &Action{Type: ActionSettlePosition},
		}

		action, evaluateErr := branch.Evaluate(nil, nil)

		convey.Convey("It should return the configured action", func() {
			convey.So(evaluateErr, convey.ShouldBeNil)
			convey.So(action, convey.ShouldNotBeNil)
			convey.So(action.Type, convey.ShouldEqual, ActionSettlePosition)
		})
	})
}
