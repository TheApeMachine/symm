package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPriorEvaluatorEvaluate(t *testing.T) {
	Convey("Given independent priors sharing one evaluator", t, func() {
		evaluator := newPriorEvaluator(8) // Eight-resolution retention fixture.
		priors := []*modelPrior{
			{evaluator: evaluator, state: NewPriorMemory()},
			{evaluator: evaluator, state: NewPriorMemory()},
		}
		references := []core.Primitive{
			NewPrior(store.NewConstant(core.From(8.0)), NewPriorMemory()),
			NewPrior(store.NewConstant(core.From(8.0)), NewPriorMemory()),
		}
		Convey("Interleaved updates and aging queries match isolated graphs exactly", func() {
			for epoch := uint64(1); epoch <= 12; epoch++ {
				for side, prior := range priors {
					input := core.Record(map[string]any{"value": float64(side*2-1) * float64(epoch), "authority": 0.5, "epoch": epoch})
					actual, err := evaluator.evaluate(prior, input)
					So(err, ShouldBeNil)
					expected, err := transport.Evaluate[map[string]core.Primitive](references[side], input)
					So(err, ShouldBeNil)
					reading, err := ProjectPrior(actual)
					So(err, ShouldBeNil)
					reference, err := ProjectPrior(expected)
					So(err, ShouldBeNil)
					So(reading, ShouldResemble, reference)
				}
			}
			for side, prior := range priors {
				input := core.Record(map[string]any{"epoch": uint64(20)})
				expected, err := transport.Evaluate[map[string]core.Primitive](references[side], input)
				So(err, ShouldBeNil)
				reference, err := ProjectPrior(expected)
				So(err, ShouldBeNil)
				So(prior.reading(20), ShouldResemble, reference)
			}
		})
	})
}

func BenchmarkPriorEvaluatorEvaluate(b *testing.B) {
	evaluator := newPriorEvaluator(8)
	priors := []*modelPrior{
		{evaluator: evaluator, state: NewPriorMemory()},
		{evaluator: evaluator, state: NewPriorMemory()},
	}
	input := core.Record(map[string]any{"value": 1.0, "authority": 0.5})
	step := 0
	b.ReportAllocs()
	for b.Loop() {
		if _, err := evaluator.evaluate(priors[step%len(priors)], input); err != nil {
			b.Fatal(err)
		}
		step++
	}
}
