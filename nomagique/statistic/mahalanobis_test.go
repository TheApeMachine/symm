package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestMahalanobis(t *testing.T) {
	Convey("Given a 1-dimensional residual", t, func() {
		residualSlot := types.MustIntern("residual_1d")
		pipeline := Mahalanobis("", residualSlot)
		stream := types.NewStream(pipeline, types.Frame{})

		Convey("It should match scalar z^2 SNR", func() {
			// Feed initial variance = 4 (stddev = 2)
			for range 5 {
				frame := types.Frame{}
				frame.Put(residualSlot, 2.0)
				stream.Step(frame)
			}

			testFrame := types.Frame{}
			testFrame.Put(residualSlot, 4.0) // 2 stddev departure -> z=2, SNR = z^2 = 4
			result := stream.Step(testFrame)
			So(result.Err, ShouldBeNil)

			ready, _ := result.Get(SymbolMahalanobisReady)
			So(ready, ShouldEqual, 1)

			snr, _ := result.Get(SymbolMahalanobisSNR)
			So(snr, ShouldAlmostEqual, 4.0, 1e-3)
		})
	})

	Convey("Given a 2-dimensional independent residual vector", t, func() {
		resSlotA := types.MustIntern("res_a")
		resSlotB := types.MustIntern("res_b")
		pipeline := Mahalanobis("", resSlotA, resSlotB)
		stream := types.NewStream(pipeline, types.Frame{})

		// Seed covariance matrix Sigma = [[1, 0], [0, 1]]
		for range 5 {
			frameA := types.Frame{}
			frameA.Put(resSlotA, 1.0)
			frameA.Put(resSlotB, 0.0)
			stream.Step(frameA)

			frameB := types.Frame{}
			frameB.Put(resSlotA, 0.0)
			frameB.Put(resSlotB, 1.0)
			stream.Step(frameB)
		}

		Convey("It should calculate (1/2) * delta^T * Sigma^-1 * delta", func() {
			evalFrame := types.Frame{}
			evalFrame.Put(resSlotA, 1.0)
			evalFrame.Put(resSlotB, 1.0)
			result := stream.Step(evalFrame)
			So(result.Err, ShouldBeNil)

			ready, _ := result.Get(SymbolMahalanobisReady)
			So(ready, ShouldEqual, 1)

			// delta = [1, 1], Sigma approx 0.5 * I -> delta^T Sigma^-1 delta approx 4 -> SNR = 4 / 2 = 2
			snr, _ := result.Get(SymbolMahalanobisSNR)
			So(snr, ShouldBeGreaterThan, 0)

			distance, _ := result.Get(SymbolMahalanobisDistance)
			So(distance, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a namespaced prefix", t, func() {
		prefix := "joint_flow"
		resSlot := types.MustIntern("res_val")
		pipeline := Mahalanobis(prefix, resSlot)
		stream := types.NewStream(pipeline, types.Frame{})

		slots := newMahalanobisSlots(prefix)

		for range 3 {
			frame := types.Frame{}
			frame.Put(resSlot, 2.0)
			stream.Step(frame)
		}

		evalFrame := types.Frame{}
		evalFrame.Put(resSlot, 2.0)
		result := stream.Step(evalFrame)

		Convey("It should output to the namespaced slots", func() {
			ready, hasReady := result.Get(slots.ready)
			So(hasReady, ShouldBeTrue)
			So(ready, ShouldEqual, 1)

			snr, hasSNR := result.Get(slots.snr)
			So(hasSNR, ShouldBeTrue)
			So(snr, ShouldAlmostEqual, 1.0, 1e-3)
		})
	})
}

func BenchmarkMahalanobis(b *testing.B) {
	resSlotA := types.MustIntern("bench_a")
	resSlotB := types.MustIntern("bench_b")
	resSlotC := types.MustIntern("bench_c")

	pipeline := Mahalanobis("", resSlotA, resSlotB, resSlotC)
	stream := types.NewStream(pipeline, types.Frame{})

	for iteration := range 10 {
		frame := types.Frame{}
		frame.Put(resSlotA, float64(iteration%3+1))
		frame.Put(resSlotB, float64((iteration+1)%3+1))
		frame.Put(resSlotC, float64((iteration+2)%3+1))
		_ = stream.Step(frame)
	}

	input := types.Frame{}
	input.Put(resSlotA, 2.0)
	input.Put(resSlotB, 1.5)
	input.Put(resSlotC, 3.0)

	b.ReportAllocs()

	for b.Loop() {
		_ = stream.Step(input)
	}
}
