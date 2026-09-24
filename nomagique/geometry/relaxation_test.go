package geometry_test

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

func TestRelaxation(t *testing.T) {
	ctx := context.Background()

	Convey("Given four points on their original lattice", t, func() {
		client := geometry.Relaxation_ServerToClient(geometry.NewRelaxation())
		defer client.Release()

		// relax takes one step on one relationship between points 0 and 3,
		// which start diagonally apart at (0,0) and (1,1).
		relax := func(positions []float64, known []bool, authority []float64, strength, distance float64) []float64 {
			So(client.Write(ctx, func(params geometry.Relaxation_write_Params) error {
				held, err := params.NewPositions(int32(len(positions)))

				if err != nil {
					return err
				}

				for offset, value := range positions {
					held.Set(offset, value)
				}

				flags, err := params.NewKnown(int32(len(known)))

				if err != nil {
					return err
				}

				for point, flag := range known {
					flags.Set(point, flag)
				}

				weights, err := params.NewAuthority(int32(len(authority)))

				if err != nil {
					return err
				}

				for point, weight := range authority {
					weights.Set(point, weight)
				}

				from, err := params.NewFromNodes(1)

				if err != nil {
					return err
				}

				to, err := params.NewToNodes(1)

				if err != nil {
					return err
				}

				strengths, err := params.NewStrength(1)

				if err != nil {
					return err
				}

				distances, err := params.NewDistance(1)

				if err != nil {
					return err
				}

				from.Set(0, 0)
				to.Set(0, 3)
				strengths.Set(0, strength)
				distances.Set(0, distance)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			placed, err := results.Positions()
			So(err, ShouldBeNil)

			out := make([]float64, placed.Len())

			for offset := range placed.Len() {
				out[offset] = placed.At(offset)
			}

			return out
		}

		Convey("Unplaced points start at their original coordinates", func() {
			positions := relax(nil, nil, []float64{0, 0, 0, 0}, 1, 0.5)
			So(positions, ShouldResemble, []float64{0, 0, 1, 0, 0, 1, 1, 1})
		})

		Convey("A sympathetic pair closes on its target, and the weak point does the travelling", func() {
			positions := relax(nil, nil, []float64{0.9, 0, 0, 0.1}, 1, 0.5)
			strong := math.Hypot(positions[0], positions[1])
			weak := math.Hypot(positions[6]-1, positions[7]-1)

			So(math.Hypot(positions[6]-positions[0], positions[7]-positions[1]), ShouldBeLessThan, math.Sqrt2)
			So(weak, ShouldBeGreaterThan, strong)
			So(weak, ShouldAlmostEqual, 9*strong)
		})

		Convey("A repelling pair is pushed apart", func() {
			positions := relax(nil, nil, []float64{0.5, 0, 0, 0.5}, -1, 2)
			So(math.Hypot(positions[6]-positions[0], positions[7]-positions[1]), ShouldBeGreaterThan, math.Sqrt2)
		})

		Convey("Points with no authority between them have no evidence to move by", func() {
			positions := relax(nil, nil, []float64{0, 0, 0, 0}, 2, 0.1)
			So(positions, ShouldResemble, []float64{0, 0, 1, 0, 0, 1, 1, 1})
		})

		Convey("Coincident points separate under repulsion", func() {
			positions := relax([]float64{0, 0, 1, 0, 0, 1, 0, 0}, []bool{true, true, true, true}, []float64{0.5, 0, 0, 0.5}, -1, 2)
			So(math.Hypot(positions[6]-positions[0], positions[7]-positions[1]), ShouldBeGreaterThan, 0)
		})
	})
}
