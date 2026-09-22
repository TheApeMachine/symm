package hawkes_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
)

/*
radiusOf drives the SpectralRadius node over one square matrix.
*/
func radiusOf(t *testing.T, matrix []float64, dimension int) float64 {
	t.Helper()
	ctx := context.Background()
	client := hawkes.SpectralRadius_ServerToClient(hawkes.NewSpectralRadius())

	err := client.Write(ctx, func(params hawkes.SpectralRadius_write_Params) error {
		if err := writeFloats(params.NewMatrix, matrix); err != nil {
			return err
		}

		params.SetDimension(int32(dimension))
		return nil
	})

	if err != nil {
		t.Fatalf("spectral radius write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("spectral radius stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("spectral radius done: %v", err)
	}

	return results.Radius()
}

func TestSpectralRadiusServer_Write(t *testing.T) {
	Convey("Given square matrices with known eigenvalues", t, func() {
		Convey("When the matrix is diagonal", func() {
			radius := radiusOf(t, []float64{0.3, 0, 0, 0.7}, 2)

			Convey("Then the radius is the largest diagonal entry", func() {
				So(math.Abs(radius-0.7), ShouldBeLessThan, 1e-12)
			})
		})

		Convey("When the eigenvalues form a complex pair", func() {
			// Rotation by a quarter turn scaled by one half: eigenvalues are
			// +/- i/2, which are invisible to the diagonal but have modulus
			// one half. A radius read off the diagonal would report zero.
			radius := radiusOf(t, []float64{0, -0.5, 0.5, 0}, 2)

			Convey("Then the radius is their modulus, not their real part", func() {
				So(math.Abs(radius-0.5), ShouldBeLessThan, 1e-12)
			})
		})

		Convey("When an eigenvalue is negative", func() {
			radius := radiusOf(t, []float64{-0.9, 0, 0, 0.2}, 2)

			Convey("Then its magnitude decides, not its sign", func() {
				So(math.Abs(radius-0.9), ShouldBeLessThan, 1e-12)
			})
		})

		Convey("When a branching matrix sits either side of criticality", func() {
			subcritical := radiusOf(t, []float64{0.4, 0.2, 0.2, 0.4}, 2)
			supercritical := radiusOf(t, []float64{0.9, 0.4, 0.4, 0.9}, 2)

			Convey("Then criticality is what separates them", func() {
				So(subcritical, ShouldBeLessThan, 1)
				So(supercritical, ShouldBeGreaterThan, 1)
			})
		})

		Convey("When the matrix is smaller than the stated dimension", func() {
			radius := radiusOf(t, []float64{0.5}, 2)

			Convey("Then nothing is reported rather than a padded matrix", func() {
				So(radius, ShouldEqual, 0)
			})
		})
	})
}
