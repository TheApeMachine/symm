package trader

import (
	"fmt"
	"testing"

	"github.com/theapemachine/datura"
	balancefixtures "github.com/theapemachine/symm/tests/fixtures/balances"

	. "github.com/smartystreets/goconvey/convey"
)

func testAllocator(fraction float64) *Allocator {
	return &Allocator{
		fraction: fraction,
		quote:    "USD",
	}
}

func allocationAction(symbol string, score, confidence float64) *datura.Artifact {
	return datura.Acquire("story", datura.APPJSON).
		WithRole("buy").
		WithScope(symbol).
		Poke(score, "decision", "score").
		Poke(confidence, "confidence")
}

func allocationBalances() *datura.Artifact {
	for artifact := range balancefixtures.NewFixture(balancefixtures.SNAPSHOT, 1).Artifacts() {
		return artifact
	}

	return nil
}

func TestAllocatorAllowed(t *testing.T) {
	Convey("Given an Allocator", t, func() {
		allocator := testAllocator(0.05)
		steps := 0

		for balances := range balancefixtures.NewFixture(balancefixtures.UPDATE, 5).Artifacts() {
			sequence := datura.Peek[int](balances, "sequence")
			balance := datura.Peek[float64](balances, "data", 0, "balance")
			steps++

			Convey(fmt.Sprintf("When update %d carries balance %.8f", sequence, balance), func() {
				Convey("When actions have decision scores", func() {
					actions := []*datura.Artifact{
						allocationAction("LOW/USD", 0.1, 0.2),
						allocationAction("HIGH/USD", 0.9, 0.6),
						allocationAction("MID/USD", 0.5, 0.4),
					}

					allowed := allocator.Allowed(actions, balances)

					Convey("It should preserve every candidate and rank them by score", func() {
						So(allowed, ShouldHaveLength, 3)
						So(datura.Peek[string](allowed[0], "scope"), ShouldEqual, "HIGH/USD")
						So(datura.Peek[string](allowed[1], "scope"), ShouldEqual, "MID/USD")
						So(datura.Peek[string](allowed[2], "scope"), ShouldEqual, "LOW/USD")
					})

					Convey("It should stamp allocation attributes on each candidate", func() {
						So(datura.Peek[bool](allowed[0], "allowed"), ShouldBeTrue)
						So(datura.Peek[float64](allowed[0], "fraction"), ShouldEqual, 0.03)
						So(datura.Peek[float64](allowed[1], "fraction"), ShouldEqual, 0.020000000000000004)
						So(datura.Peek[float64](allowed[2], "fraction"), ShouldEqual, 0.010000000000000002)
					})
				})

				Convey("When balances include an already-held asset", func() {
					actions := []*datura.Artifact{
						allocationAction("BTC/USD", 0.9, 0.6),
						allocationAction("ETH/USD", 0.8, 0.5),
					}

					allowed := allocator.Allowed(actions, balances)

					Convey("It should not gate candidates on holdings", func() {
						So(allowed, ShouldHaveLength, 2)
						So(datura.Peek[string](allowed[0], "scope"), ShouldEqual, "BTC/USD")
						So(datura.Peek[string](allowed[1], "scope"), ShouldEqual, "ETH/USD")
					})
				})

				Convey("When balances arrive as an update sequence", func() {
					actions := []*datura.Artifact{
						allocationAction("BTC/USD", 0.9, 0.6),
						allocationAction("ETH/USD", 0.8, 0.5),
					}

					allowed := allocator.Allowed(actions, balances)

					So(datura.Peek[string](balances, "type"), ShouldEqual, "update")
					So(allowed, ShouldHaveLength, 2)
					So(datura.Peek[string](allowed[0], "scope"), ShouldEqual, "BTC/USD")
					So(datura.Peek[bool](allowed[0], "allowed"), ShouldBeTrue)
					So(datura.Peek[float64](allowed[0], "fraction"), ShouldEqual, 0.03)
				})
			})
		}

		Convey("It should test every generated balance update", func() {
			So(steps, ShouldEqual, 5)
		})
	})
}

func TestAllocatorCalculate(t *testing.T) {
	Convey("Given Allocator.calculate", t, func() {
		allocator := testAllocator(0.05)

		Convey("When confidence is positive", func() {
			Convey("It should scale the base fraction by confidence", func() {
				So(allocator.calculate(0.6), ShouldEqual, 0.03)
			})
		})

		Convey("When confidence is zero", func() {
			Convey("It should return zero allocation", func() {
				So(allocator.calculate(0), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkAllocatorAllowed(benchmark *testing.B) {
	allocator := testAllocator(0.05)
	balances := allocationBalances()

	benchmark.ReportAllocs()
	benchmark.ResetTimer()

	for index := 0; index < benchmark.N; index++ {
		actions := make([]*datura.Artifact, 16)

		for actionIndex := range actions {
			actions[actionIndex] = allocationAction(
				fmt.Sprintf("ASSET%d/USD", actionIndex),
				float64(len(actions)-actionIndex),
				0.5,
			)
		}

		allocator.Allowed(actions, balances)
	}
}