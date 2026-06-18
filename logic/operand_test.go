package logic

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool")

func TestNewTree(t *testing.T) {
	convey.Convey("Given embedded tree rules", t, func() {
		pool := qpool.NewQ[any](context.Background(), 2, 4, nil)
		tree, err := NewTree(context.Background(), pool)

		convey.Convey("It should decode without error", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(tree, convey.ShouldNotBeNil)
			convey.So(len(tree.Branches), convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestConditionOperandEvaluate(t *testing.T) {
	convey.Convey("Given flat playbook operands", t, func() {
		measurements := []Measurement{
			{
				Source:     SourceExhaustion,
				Symbol:     "SOL/EUR",
				Category:   CategoryMechanicalCollapse,
				Confidence: 0.62,
			},
			{
				Source:     SourceFluid,
				Symbol:     "SOL/EUR",
				Category:   CategoryLaminar,
				Confidence: 0.41,
			},
		}

		holdings := &Balances{
			Inventory: map[string]float64{"SOL/EUR": 1.5},
		}

		convey.Convey("It should match held inventory", func() {
			held, heldErr := (&Condition{
				Type: ConditionIsTrue,
				Left: ConditionOperand{
					Type:    SubjectHolding,
					Holding: &HoldingRef{Held: true},
				},
			}).Evaluate(measurements, holdings)

			convey.So(heldErr, convey.ShouldBeNil)
			convey.So(held, convey.ShouldBeTrue)
		})

		convey.Convey("It should match source category predicates", func() {
			matched, matchedErr := (&Condition{
				Type: ConditionIsTrue,
				Left: ConditionOperand{
					Source:   SourceExhaustion,
					Type:     SubjectCategory,
					Category: &Category{Type: CategoryMechanicalCollapse},
				},
			}).Evaluate(measurements, holdings)

			convey.So(matchedErr, convey.ShouldBeNil)
			convey.So(matched, convey.ShouldBeTrue)
		})

		convey.Convey("It should treat missing source confidence as unsatisfied", func() {
			value, valueErr := (&ConditionOperand{
				Source: SourcePumpDump,
				Type:   SubjectConfidence,
			}).resolve(measurements, holdings)

			convey.So(valueErr, convey.ShouldBeNil)
			convey.So(value, convey.ShouldEqual, -1)
		})

		convey.Convey("It should compare confidence against dynamic baselines", func() {
			matched, matchedErr := (&Condition{
				Type: ConditionIsGreaterThanOrEqual,
				Left: ConditionOperand{
					Source: SourceExhaustion,
					Type:   SubjectConfidence,
				},
				Right: ConditionOperand{
					Type:       SubjectConfidence,
					Confidence: ConfidenceExitBaseline,
				},
			}).Evaluate(measurements, holdings)

			convey.So(matchedErr, convey.ShouldBeNil)
			convey.So(matched, convey.ShouldBeTrue)

			exitBaseline, exitErr := confidenceBaseline(
				measurements,
				ConfidenceExitBaseline,
			)
			entryBaseline, entryErr := confidenceBaseline(
				measurements,
				ConfidenceEntryBaseline,
			)

			convey.So(exitErr, convey.ShouldBeNil)
			convey.So(entryErr, convey.ShouldBeNil)
			convey.So(entryBaseline, convey.ShouldBeGreaterThan, exitBaseline)
		})

		convey.Convey("It should keep confidence baseline at the top without samples", func() {
			baseline, baselineErr := confidenceBaseline(nil, ConfidenceEntryBaseline)

			convey.So(baselineErr, convey.ShouldBeNil)
			convey.So(baseline, convey.ShouldEqual, 1)
		})
	})
}

func BenchmarkConfidenceBaseline(b *testing.B) {
	measurements := []Measurement{
		{Source: SourceExhaustion, Confidence: 0.62},
		{Source: SourceFluid, Confidence: 0.41},
		{Source: SourceCVD, Confidence: 0.55},
		{Source: SourcePumpDump, Confidence: 0.48},
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = confidenceBaseline(measurements, ConfidenceEntryBaseline)
	}
}
