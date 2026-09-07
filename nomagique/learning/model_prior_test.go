package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestModelPriorReading(t *testing.T) {
	Convey("Given independent typed priors and isolated Primitive connections", t, func() {
		priors := []modelPrior{{memory: 8}, {memory: 8}}
		references := []core.Primitive{
			NewPrior(store.NewConstant(core.From(8.0)), NewPriorMemory()),
			NewPrior(store.NewConstant(core.From(8.0)), NewPriorMemory()),
		}
		Convey("Interleaved signed updates and dormant queries retain independent evidence", func() {
			for epoch := uint64(1); epoch <= 12; epoch++ {
				for side := range priors {
					value := float64(side*2-1) * float64(epoch)
					So(priors[side].state.Observe(value, 0.5, priors[side].memory, epoch), ShouldBeNil)
					fields, err := transport.Evaluate[map[string]core.Primitive](references[side],
						core.Record(map[string]any{"value": value, "authority": 0.5, "epoch": epoch}))
					So(err, ShouldBeNil)
					expected, err := ProjectPrior(fields)
					So(err, ShouldBeNil)
					So(priors[side].reading(epoch), ShouldResemble, expected)
				}
			}
			for _, epoch := range []uint64{20, 1_000_000, 1_000_000} {
				for side := range priors {
					fields, err := transport.Evaluate[map[string]core.Primitive](references[side], core.Record(map[string]any{"epoch": epoch}))
					So(err, ShouldBeNil)
					expected, err := ProjectPrior(fields)
					So(err, ShouldBeNil)
					So(priors[side].reading(epoch), ShouldResemble, expected)
				}
			}
		})
	})
}

func BenchmarkModelPriorReading(b *testing.B) {
	prior := modelPrior{memory: 8}

	if err := prior.state.Observe(-1, 0.5, prior.memory, 1); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()

	for b.Loop() {
		prior.reading(1)
	}
}
