package numeric

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShiftPriceSamplesInto(t *testing.T) {
	Convey("Given reusable storage and price samples", t, func() {
		start := time.Now()
		samples := []PriceSample{
			{At: start, Price: 100},
			{At: start.Add(time.Second), Price: 101},
		}
		buffer := make([]PriceSample, 0, len(samples))

		shifted := ShiftPriceSamplesInto(buffer, samples, time.Minute)

		Convey("It should shift timestamps without changing prices", func() {
			So(len(shifted), ShouldEqual, len(samples))
			So(cap(shifted), ShouldEqual, cap(buffer))
			So(shifted[0].At, ShouldResemble, samples[0].At.Add(time.Minute))
			So(shifted[1].Price, ShouldEqual, samples[1].Price)
		})

		Convey("The allocating wrapper should match the reusable path", func() {
			So(ShiftPriceSamples(samples, time.Minute), ShouldResemble, shifted)
		})
	})
}

func BenchmarkShiftPriceSamplesInto(benchmark *testing.B) {
	now := time.Now()
	samples := make([]PriceSample, 128)

	for index := range samples {
		samples[index] = PriceSample{
			At:    now.Add(time.Duration(index) * time.Second),
			Price: 100 + float64(index),
		}
	}

	buffer := make([]PriceSample, 0, len(samples))

	for benchmark.Loop() {
		_ = ShiftPriceSamplesInto(buffer, samples, time.Minute)
	}
}
