package statistic

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWeightedWrite(t *testing.T) {
	Convey("Given observations carrying unequal authority", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		client := Weighted_ServerToClient(NewWeighted(ctx))
		defer client.Release()

		summarise := func(values, weights []float64) Weighted_done_Results {
			err := client.Write(ctx, func(params Weighted_write_Params) error {
				carried, err := params.NewValue(int32(len(values)))

				if err != nil {
					return err
				}

				for index, value := range values {
					carried.Set(index, value)
				}

				authority, err := params.NewWeight(int32(len(weights)))

				if err != nil {
					return err
				}

				for index, weight := range weights {
					authority.Set(index, weight)
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

		Convey("When one observation rests on far more evidence than the others", func() {
			results := summarise([]float64{1, 0, 0}, []float64{98, 1, 1})

			Convey("Then it decides the mean", func() {
				So(results.Mean(), ShouldAlmostEqual, 0.98, 1e-12)
			})

			Convey("Then the set is worth fewer observations than arrived", func() {
				// Three arrived, but the weights are so lopsided the set is
				// worth barely more than one equally weighted observation.
				So(results.Count(), ShouldEqual, 3)
				So(results.Effective(), ShouldBeLessThan, 1.1)
				So(results.Total(), ShouldEqual, 100)
			})
		})

		Convey("When every observation carries the same authority", func() {
			results := summarise([]float64{2, 4, 6}, []float64{1, 1, 1})

			Convey("Then it is the ordinary mean and the set is worth them all", func() {
				So(results.Mean(), ShouldAlmostEqual, 4, 1e-12)
				So(results.Effective(), ShouldAlmostEqual, 3, 1e-12)
			})

			Convey("Then the spread is the ordinary variance", func() {
				So(results.Variance(), ShouldAlmostEqual, 8.0/3.0, 1e-12)
				So(math.Sqrt(results.Variance()), ShouldBeGreaterThan, 1.6)
			})
		})

		Convey("When a value arrives without the authority behind it", func() {
			err := client.Write(ctx, func(params Weighted_write_Params) error {
				carried, err := params.NewValue(2)

				if err != nil {
					return err
				}

				carried.Set(0, 1)
				carried.Set(1, 2)

				authority, err := params.NewWeight(1)

				if err != nil {
					return err
				}

				authority.Set(0, 1)
				return nil
			})
			So(err, ShouldBeNil)

			Convey("Then the mismatch is reported rather than summarised over", func() {
				So(client.WaitStreaming(), ShouldNotBeNil)
			})
		})
	})
}
