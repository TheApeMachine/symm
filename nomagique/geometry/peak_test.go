package geometry_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/geometry"
)

/*
edge is one relationship read by Peak.
*/
type edge struct {
	from, to int64
	strength float64
}

func TestPeak(t *testing.T) {
	ctx := context.Background()

	Convey("Given five points in a row, half a cell apart, whose authority rises to both ends", t, func() {
		client := geometry.Peak_ServerToClient(geometry.NewPeak())
		defer client.Release()

		positions := []float64{0, 0, 0.5, 0, 1, 0, 1.5, 0, 2, 0}
		heights := []float64{1, 0.5, 0.25, 0.5, 1}
		var state []float64
		var vocabularies []string

		// drain reads the partition once, carrying its record forward the way
		// its store does, and returns the regions when settled, nil otherwise.
		drain := func(points []float64, authority []float64, edges []edge) []string {
			So(client.Write(ctx, func(params geometry.Peak_write_Params) error {
				placed, err := params.NewPositions(int32(len(points)))

				if err != nil {
					return err
				}

				for offset, value := range points {
					placed.Set(offset, value)
				}

				weights, err := params.NewAuthority(int32(len(authority)))

				if err != nil {
					return err
				}

				for point, height := range authority {
					weights.Set(point, height)
				}

				from, err := params.NewFromNodes(int32(len(edges)))

				if err != nil {
					return err
				}

				to, err := params.NewToNodes(int32(len(edges)))

				if err != nil {
					return err
				}

				strength, err := params.NewStrength(int32(len(edges)))

				if err != nil {
					return err
				}

				for position, relation := range edges {
					from.Set(position, relation.from)
					to.Set(position, relation.to)
					strength.Set(position, relation.strength)
				}

				prior, err := params.NewPrior(int32(len(state)))

				if err != nil {
					return err
				}

				for offset, value := range state {
					prior.Set(offset, value)
				}

				known, err := params.NewKnown(int32(len(state) / 4))

				if err != nil {
					return err
				}

				for point := range len(state) / 4 {
					known.Set(point, true)
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			record, err := results.State()
			So(err, ShouldBeNil)
			state = make([]float64, record.Len())

			for offset := range record.Len() {
				state[offset] = record.At(offset)
			}

			if results.Which() != geometry.Watershed_Which_settled {
				return nil
			}

			region, err := results.Settled().Regions()
			So(err, ShouldBeNil)
			vocabulary, err := results.Settled().Vocabulary()
			So(err, ShouldBeNil)
			So(len(vocabulary), ShouldEqual, 64)
			vocabularies = append(vocabularies, string(vocabulary))

			names := []string{}

			for point := range region.Len() {
				name, err := region.At(point)
				So(err, ShouldBeNil)
				names = append(names, name)
			}

			return names
		}

		chain := []edge{{0, 1, 1}, {1, 2, 1}, {2, 3, 1}, {3, 4, 1}}

		Convey("With no previous partition the map is still moving and publishes no regions", func() {
			So(drain(positions, heights, chain), ShouldBeNil)

			Convey("A partition seen once has not yet held", func() {
				So(drain(positions, heights, nil), ShouldBeNil)

				Convey("Once it has held, every point drains to its peak and the border falls where the weakest points meet", func() {
					So(drain(positions, heights, nil), ShouldResemble, []string{"0", "0", "0", "4", "4"})
				})
			})
		})

		Convey("Points that are near but not sympathetic do not drain into each other", func() {
			drain(positions, heights, []edge{{0, 1, -1}, {1, 2, 1}, {2, 3, 1}, {3, 4, 1}})
			drain(positions, heights, nil)
			So(drain(positions, heights, nil), ShouldResemble, []string{"0", "1", "1", "4", "4"})
		})

		Convey("Sympathetic points the arrangement has left a cell apart do not drain into each other", func() {
			spread := []float64{0, 0, 1, 0, 2, 0, 3, 0, 4, 0}
			drain(spread, heights, chain)
			drain(spread, heights, nil)
			So(drain(spread, heights, nil), ShouldResemble, []string{"0", "1", "2", "3", "4"})
		})

		Convey("Regions are read from the partition in force when the evaluation begins", func() {
			drain(positions, heights, chain)
			drain(positions, heights, nil)
			So(drain(positions, heights, nil), ShouldResemble, []string{"0", "0", "0", "4", "4"})
			So(drain(positions, heights, []edge{{3, 4, -1}}), ShouldResemble, []string{"0", "0", "0", "4", "4"})

			Convey("And while the arrangement then moves, the partition that held stands", func() {
				So(drain(positions, heights, nil), ShouldResemble, []string{"0", "0", "0", "4", "4"})

				Convey("Until the new partition has held, under a new vocabulary", func() {
					So(drain(positions, heights, nil), ShouldResemble, []string{"0", "0", "0", "3", "4"})
					So(vocabularies[len(vocabularies)-1], ShouldNotEqual, vocabularies[0])
					So(vocabularies[1], ShouldEqual, vocabularies[0])
				})
			})
		})
	})
}
