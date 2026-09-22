package hawkes_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
intensityOf drives the Intensity node over one parameter set and support.
*/
func intensityOf(t *testing.T, baseline, excitation, support []float64, dimension int) []float64 {
	t.Helper()
	ctx := context.Background()
	client := hawkes.Intensity_ServerToClient(hawkes.NewIntensity())

	err := client.Write(ctx, func(params hawkes.Intensity_write_Params) error {
		if err := writeFloats(params.NewBaseline, baseline); err != nil {
			return err
		}

		if err := writeFloats(params.NewExcitation, excitation); err != nil {
			return err
		}

		if err := writeFloats(params.NewSupport, support); err != nil {
			return err
		}

		params.SetDimension(int32(dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("intensity write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("intensity stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("intensity done: %v", err)
	}

	list, err := results.Intensity()

	if err != nil {
		t.Fatalf("intensity list: %v", err)
	}

	return readList(list.Len(), list.At)
}

func TestIntensityServer_Write(t *testing.T) {
	Convey("Given a baseline, an excitation matrix, and standing support", t, func() {
		baseline := []float64{0.5, 0.25}

		Convey("When only the first component has any support standing", func() {
			// The excitation matrix is read as entry (k, j) being component
			// j's influence on component k. With support only on component
			// 0, the answer separates the matrix's two orientations: a
			// transposed read would move 0.7 onto component 0 instead.
			excitation := []float64{0.1, 0.2, 0.7, 0.3}
			intensity := intensityOf(t, baseline, excitation, []float64{2, 0}, 2)

			Convey("Then each component is driven by its own column of the matrix", func() {
				So(math.Abs(intensity[0]-(0.5+0.1*2)), ShouldBeLessThan, 1e-12)
				So(math.Abs(intensity[1]-(0.25+0.7*2)), ShouldBeLessThan, 1e-12)
			})
		})

		Convey("When no support is standing", func() {
			intensity := intensityOf(t, baseline, []float64{0.1, 0.2, 0.7, 0.3}, []float64{0, 0}, 2)

			Convey("Then the process runs at its baseline", func() {
				So(intensity, ShouldResemble, baseline)
			})
		})

		Convey("When the excitation matrix is smaller than the stated dimension", func() {
			intensity := intensityOf(t, baseline, []float64{0.1, 0.2}, []float64{1, 1}, 2)

			Convey("Then nothing is reported rather than a padded matrix", func() {
				So(intensity, ShouldBeEmpty)
			})
		})

		Convey("When the support is shorter than the stated dimension", func() {
			intensity := intensityOf(t, baseline, []float64{0.1, 0.2, 0.7, 0.3}, []float64{1}, 2)

			Convey("Then the missing component is not read as zero support", func() {
				So(intensity, ShouldBeEmpty)
			})
		})
	})
}
