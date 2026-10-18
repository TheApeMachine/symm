package tokenizer

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFastWindow(t *testing.T) {
	Convey("Given a FastWindow of size 5", t, func() {
		window := NewFastWindow(5)
		So(window, ShouldNotBeNil)

		Convey("When it is empty", func() {
			Convey("Stats should return 0, 0", func() {
				mean, stddev := window.Stats()
				So(mean, ShouldEqual, 0)
				So(stddev, ShouldEqual, 0)
			})
			Convey("Warmed should be false", func() {
				So(window.Warmed(), ShouldBeFalse)
			})
		})

		Convey("When partially filled", func() {
			window.Push(2.0)
			window.Push(4.0)

			Convey("Warmed should be false", func() {
				So(window.Warmed(), ShouldBeFalse)
			})

			Convey("Stats should return expected mean and stddev", func() {
				mean, stddev := window.Stats()
				So(mean, ShouldEqual, 3.0)
				// Variance = sum_sq_diff / (N-1) = ((2-3)^2 + (4-3)^2) / 1 = 2
				// Stddev = sqrt(2)
				So(math.Abs(stddev-math.Sqrt(2.0)), ShouldBeLessThan, 1e-6)
			})
		})

		Convey("When fully filled and evicting", func() {
			// Push 1, 2, 3, 4, 5
			for i := 1.0; i <= 5.0; i++ {
				window.Push(i)
			}

			Convey("Warmed should be true", func() {
				So(window.Warmed(), ShouldBeTrue)
			})

			Convey("Stats should reflect the 5 elements", func() {
				mean, stddev := window.Stats()
				So(mean, ShouldEqual, 3.0)
				// Var for {1, 2, 3, 4, 5} is 2.5, Stddev is sqrt(2.5) ≈ 1.5811388
				So(math.Abs(stddev-math.Sqrt(2.5)), ShouldBeLessThan, 1e-6)
			})

			Convey("Subsequent pushes should evict oldest", func() {
				window.Push(6.0)
				// Window is now {2, 3, 4, 5, 6}. Mean = 4.0, Var = 2.5
				mean, stddev := window.Stats()
				So(mean, ShouldEqual, 4.0)
				So(math.Abs(stddev-math.Sqrt(2.5)), ShouldBeLessThan, 1e-6)
			})
		})

		Convey("When simulating a push", func() {
			window.Push(2.0)
			window.Push(4.0)

			simMean, simStddev := window.SimulatePush(6.0)

			Convey("It should return stats as if pushed", func() {
				So(simMean, ShouldEqual, 4.0)
				// Var for {2, 4, 6} is 4. Stddev = 2
				So(math.Abs(simStddev-2.0), ShouldBeLessThan, 1e-6)
			})

			Convey("It should not modify the actual window state", func() {
				mean, stddev := window.Stats()
				So(mean, ShouldEqual, 3.0)
				So(math.Abs(stddev-math.Sqrt(2.0)), ShouldBeLessThan, 1e-6)
			})
		})

		Convey("When triggering recalibration", func() {
			// Trigger recalibration by pushing size*2 times
			for i := 0; i < 10; i++ {
				window.Push(float64(i))
			}

			Convey("Drifts counter should be reset", func() {
				So(window.drifts, ShouldEqual, 0)
			})

			Convey("Stats should remain correct", func() {
				mean, stddev := window.Stats()
				// The last 5 elements are {5, 6, 7, 8, 9}. Mean = 7.0, Var = 2.5
				So(mean, ShouldEqual, 7.0)
				So(math.Abs(stddev-math.Sqrt(2.5)), ShouldBeLessThan, 1e-6)
			})
		})
	})
	
	Convey("Given NewFastWindow with invalid size", t, func() {
		Convey("It should return nil for size 0", func() {
			window := NewFastWindow(0)
			So(window, ShouldBeNil)
		})
		Convey("It should return nil for negative size", func() {
			window := NewFastWindow(-5)
			So(window, ShouldBeNil)
		})
	})
}

func BenchmarkFastWindowPush(b *testing.B) {
	window := NewFastWindow(128)
	
	for i := 0; b.Loop(); i++ {
		window.Push(float64(i))
	}
}

func BenchmarkFastWindowSimulatePush(b *testing.B) {
	window := NewFastWindow(128)
	for i := 0; i < 128; i++ {
		window.Push(float64(i))
	}
	
	for i := 0; b.Loop(); i++ {
		window.SimulatePush(float64(i))
	}
}
