package statistic

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOrderWrite(t *testing.T) {
	Convey("Given a cross section of returns", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := Order_ServerToClient(NewOrder(ctx))
		defer client.Release()

		observe := func(values []float64) Order_done_Results {
			err := client.Write(ctx, func(params Order_write_Params) error {
				carried, err := params.NewValue(int32(len(values)))

				if err != nil {
					return err
				}

				for index, value := range values {
					carried.Set(index, value)
				}

				return nil
			})
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			t.Cleanup(release)

			results, err := future.Struct()
			So(err, ShouldBeNil)

			return results
		}

		Convey("When one member moves far more than the rest", func() {
			results := observe([]float64{0.01, 0.01, 0.01, 0.01, 0.90})

			Convey("Then the middle of the cross section is unmoved by it", func() {
				// A mean would read 0.188 and describe a cross section that
				// does not exist; four of the five members sit at 0.01.
				So(results.Median(), ShouldAlmostEqual, 0.01, 1e-12)
			})

			Convey("Then the outlier is reported as the extreme, with its sign", func() {
				So(results.ExtremeMagnitude(), ShouldAlmostEqual, 0.90, 1e-12)
				So(results.ExtremeSigned(), ShouldAlmostEqual, 0.90, 1e-12)
			})
		})

		Convey("When the cross section is split in direction", func() {
			results := observe([]float64{0.02, 0.01, 0, -0.01, -0.03})

			Convey("Then breadth is counted by direction, not by size", func() {
				So(results.Count(), ShouldEqual, 5)
				So(results.Positive(), ShouldEqual, 2)
				So(results.Negative(), ShouldEqual, 2)
				So(results.Zero(), ShouldEqual, 1)
			})

			Convey("Then the extreme keeps the sign of the member that moved most", func() {
				So(results.ExtremeSigned(), ShouldAlmostEqual, -0.03, 1e-12)
			})
			Convey("Magnitude and dispersion describe the whole current cohort", func() {
				So(results.SumAbsolute(), ShouldAlmostEqual, 0.07)
				So(results.MeanAbsolute(), ShouldAlmostEqual, 0.014)
				So(results.Rms(), ShouldAlmostEqual, math.Sqrt(0.0015/5))
				So(results.MedianDeviation(), ShouldAlmostEqual, 0.01)
				So(results.MagnitudeDeviation(), ShouldAlmostEqual, 0.01)
			})
		})
		Convey("Replacing the cohort does not retain the previous cohort's energy", func() {
			observe([]float64{1, -2, 3})
			results := observe([]float64{0.5, 0.5, 0.5})
			So(results.SumAbsolute(), ShouldEqual, 1.5)
			So(results.MeanAbsolute(), ShouldEqual, 0.5)
			So(results.Rms(), ShouldEqual, 0.5)
			So(results.MedianDeviation(), ShouldEqual, 0)
			So(results.MagnitudeDeviation(), ShouldEqual, 0)
		})

		Convey("When the spread of the cross section is measured", func() {
			results := observe([]float64{1, 2, 3, 4, 5})

			Convey("Then the quartiles bracket the middle half", func() {
				So(results.LowerQuartile(), ShouldAlmostEqual, 2, 1e-12)
				So(results.UpperQuartile(), ShouldAlmostEqual, 4, 1e-12)
				So(results.Interquartile(), ShouldAlmostEqual, 2, 1e-12)
			})
		})

		Convey("When nothing has been observed", func() {
			results := observe(nil)

			Convey("Then the cross section reports as empty rather than as zeros", func() {
				So(results.Count(), ShouldEqual, 0)
			})
		})
	})
}

/* BenchmarkOrderWrite includes publication of a 1,001-market cross section. */
func BenchmarkOrderWrite(b *testing.B) {
	ctx := context.Background()
	client := Order_ServerToClient(NewOrder(ctx))
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(ctx, func(args Order_write_Params) error {
			values, err := args.NewValue(1001)
			if err != nil {
				return err
			}
			for index := range values.Len() {
				values.Set(index, float64(index-500)/10000)
			}
			return nil
		}); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}

/*
A search that returns its best candidate says nothing about whether that
candidate was a peak. These describe how far it stood out and how sharply it
fell away, which is what separates a real one from an arbitrary choice.
*/
func TestOrderPeak(t *testing.T) {
	Convey("Given the results of a search across candidates", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := Order_ServerToClient(NewOrder(ctx))
		defer client.Release()

		search := func(values []float64) Order_done_Results {
			err := client.Write(ctx, func(params Order_write_Params) error {
				carried, err := params.NewValue(int32(len(values)))

				if err != nil {
					return err
				}

				for index, value := range values {
					carried.Set(index, value)
				}

				return nil
			})
			So(err, ShouldBeNil)

			future, release := client.Done(ctx, nil)
			t.Cleanup(release)

			results, err := future.Struct()
			So(err, ShouldBeNil)

			return results
		}

		Convey("When one candidate clearly wins", func() {
			results := search([]float64{0.1, 0.2, 0.9, 0.2, 0.1})

			Convey("Then which one it was is reported", func() {
				So(results.ExtremeIndex(), ShouldEqual, 2)
			})

			Convey("Then it stands well clear of the rest", func() {
				So(results.ExtremeProminence(), ShouldBeGreaterThan, 0.7)
			})

			Convey("Then it falls away sharply on both sides", func() {
				So(results.ExtremeCurvature(), ShouldAlmostEqual, -1.4, 1e-12)
			})
		})

		Convey("When the field is flat and the winner only edged it", func() {
			results := search([]float64{0.50, 0.50, 0.51, 0.50, 0.50})

			Convey("Then the choice is reported as barely standing out", func() {
				// The same search returns a best candidate either way; only
				// prominence says the choice was close to arbitrary.
				So(results.ExtremeProminence(), ShouldBeLessThan, 0.02)
				So(results.ExtremeCurvature(), ShouldBeGreaterThan, -0.05)
			})
		})
	})
}
