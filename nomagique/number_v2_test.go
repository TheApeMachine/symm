package nomagique

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
)

func TestNumberV2Carrier(t *testing.T) {
	Convey("Carrier participates natively in Go arithmetic without unboxing", t, func() {
		var number Scalar = 100.0
		number += 1.0
		So(number, ShouldEqual, 101.0)

		scaled := number * 0.25
		So(scaled, ShouldEqual, 25.25)

		squareRoot := math.Sqrt(float64(number))
		So(squareRoot, ShouldAlmostEqual, 10.0498756, 0.00001)

		identity := Identity{}
		stepped := number.Through(identity)
		So(stepped, ShouldEqual, 101.0)
	})

	Convey("Identity node passes carrier unchanged", t, func() {
		identity := Identity{}
		input := Scalar(42.0)
		So(identity.Step(input), ShouldEqual, 42.0)
	})
}

func TestDegenerateZeroValues(t *testing.T) {
	Convey("Degenerate Zero-Value Matrix (Table 5.1 Equivalences)", t, func() {
		input := Scalar(42.0)

		Convey("Chain with omitted slots is transparent identity I(x) = x", func() {
			emptyChain := &Chain{}
			So(emptyChain.Step(input), ShouldEqual, input)

			partialChain := &Chain{
				B: Identity{},
			}
			So(partialChain.Step(input), ShouldEqual, input)
		})

		Convey("Split with omitted slots contributes 0 (additive identity)", func() {
			emptySplit := &Split{}
			So(emptySplit.Step(input), ShouldEqual, 0.0)
		})

		Convey("Sum with omitted slots contributes 0", func() {
			emptySum := &Sum{}
			So(emptySum.Step(input), ShouldEqual, 0.0)

			partialSum := &Sum{
				A: Identity{},
			}
			So(partialSum.Step(input), ShouldEqual, input)
		})

		Convey("Product with omitted slots contributes 1 (multiplicative identity)", func() {
			emptyProduct := &Product{}
			So(emptyProduct.Step(input), ShouldEqual, 1.0)

			partialProduct := &Product{
				A: Identity{},
			}
			So(partialProduct.Step(input), ShouldEqual, input)
		})

		Convey("Decay with Rate omitted drops instantly to 0", func() {
			decay := &Decay{}
			So(decay.Step(input), ShouldEqual, 0.0)
		})

		Convey("Decay with Shape omitted decays linearly", func() {
			decay := &Decay{
				Rate: &fixedRate{rate: 0.25},
			}
			// First tick initializes
			first := decay.Step(100.0)
			So(first, ShouldEqual, 100.0)

			// Second tick: factor = 1 - 0.25 = 0.75; 100 * 0.75 + 10 = 85
			second := decay.Step(10.0)
			So(second, ShouldEqual, 85.0)
		})

		Convey("Standardize with Center omitted subtracts 0", func() {
			standardize := &Standardize{
				Scale: &fixedRate{rate: 2.0},
			}
			So(standardize.Step(10.0), ShouldEqual, 5.0)
		})

		Convey("Standardize with Scale omitted divides by 1", func() {
			standardize := &Standardize{
				Center: &fixedRate{rate: 3.0},
			}
			So(standardize.Step(10.0), ShouldEqual, 7.0)
		})
	})
}

func TestStoreAndSink(t *testing.T) {
	Convey("Law of Sinks: Store without Reduce returns 0 and does not corrupt Split", t, func() {
		store := &Store{
			Type:     DynamicRing,
			Adaptive: adaptive.Window{Type: adaptive.ADWIN},
		}

		split := &Split{
			A: Identity{},
			B: store,
		}

		result := split.Step(10.0)
		// Identity returns 10, Store returns 0 => 10 + 0 = 10
		So(result, ShouldEqual, 10.0)
		So(store.Len(), ShouldEqual, 1)
		So(store.Values()[0], ShouldEqual, 10.0)
	})

	Convey("Store with Reduce returns the reduction over the emergent window", t, func() {
		store := &Store{
			Type:     DynamicRing,
			Adaptive: adaptive.Window{Type: adaptive.ADWIN},
			Reduce:   Average,
		}

		store.Step(10.0)
		store.Step(20.0)
		out := store.Step(30.0)
		So(out, ShouldEqual, 20.0)
	})
}

func TestZeroSteadyStateAllocations(t *testing.T) {
	Convey("Chain and Split execute with 0 allocations in steady state", t, func() {
		pipeline := &Chain{
			A: &Split{
				A: Identity{},
				B: &Decay{
					Rate:  &fixedRate{rate: 0.1},
					Shape: Exponential{},
				},
			},
			B: &Standardize{
				Center: &fixedRate{rate: 5.0},
				Scale:  &fixedRate{rate: 2.0},
			},
		}

		// Warmup
		for iteration := 0; iteration < 100; iteration++ {
			_ = pipeline.Step(Scalar(iteration))
		}

		allocations := testing.AllocsPerRun(1000, func() {
			_ = pipeline.Step(15.0)
		})

		So(allocations, ShouldEqual, 0)
	})
}

type fixedRate struct {
	rate Scalar
}

func (fixed *fixedRate) Step(number Scalar) Scalar {
	return fixed.rate
}

func BenchmarkChainStep(b *testing.B) {
	pipeline := &Chain{
		A: Identity{},
		B: &fixedRate{rate: 1.0},
		C: Identity{},
	}

	input := Scalar(42.0)
	b.ResetTimer()
	b.ReportAllocs()

	for iteration := 0; iteration < b.N; iteration++ {
		_ = pipeline.Step(input)
	}
}

func BenchmarkSplitStep(b *testing.B) {
	pipeline := &Split{
		A: Identity{},
		B: &fixedRate{rate: 2.0},
	}

	input := Scalar(42.0)
	b.ResetTimer()
	b.ReportAllocs()

	for iteration := 0; iteration < b.N; iteration++ {
		_ = pipeline.Step(input)
	}
}
