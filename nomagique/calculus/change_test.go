package calculus_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/calculus"
)

func TestChange(t *testing.T) {
	ctx := context.Background()

	Convey("Given a list read against its previous reading", t, func() {
		client := calculus.Change_ServerToClient(calculus.NewChange())
		defer client.Release()

		read := func(value []float64, present []bool, previous []float64, known []bool) ([]float64, []bool, []int64, []float64) {
			So(client.Write(ctx, func(params calculus.Change_write_Params) error {
				values, err := params.NewValue(int32(len(value)))

				if err != nil {
					return err
				}

				flags, err := params.NewPresent(int32(len(present)))

				if err != nil {
					return err
				}

				for element := range value {
					values.Set(element, value[element])
					flags.Set(element, present[element])
				}

				priors, err := params.NewPrevious(int32(len(previous)))

				if err != nil {
					return err
				}

				knowns, err := params.NewKnown(int32(len(known)))

				if err != nil {
					return err
				}

				for element := range previous {
					priors.Set(element, previous[element])
					knowns.Set(element, known[element])
				}

				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)

			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			change, err := results.Change()
			So(err, ShouldBeNil)
			defined, err := results.Defined()
			So(err, ShouldBeNil)
			indices := []int64{}
			readings := []float64{}

			if results.Which() == calculus.Changed_Which_read {
				index, err := results.Read().Index()
				So(err, ShouldBeNil)
				latest, err := results.Read().Latest()
				So(err, ShouldBeNil)

				for position := range index.Len() {
					indices = append(indices, index.At(position))
					readings = append(readings, latest.At(position))
				}
			}

			moved := make([]float64, change.Len())
			flags := make([]bool, defined.Len())

			for element := range change.Len() {
				moved[element] = change.At(element)
				flags[element] = defined.At(element)
			}

			return moved, flags, indices, readings
		}

		Convey("A first reading has no change, but is what the next one is measured against", func() {
			change, defined, index, latest := read([]float64{5, 7}, []bool{true, true}, nil, nil)
			So(change, ShouldResemble, []float64{0, 0})
			So(defined, ShouldResemble, []bool{false, false})
			So(index, ShouldResemble, []int64{0, 1})
			So(latest, ShouldResemble, []float64{5, 7})
		})

		Convey("Only elements read now and known before moved; an unmoved one moved by exactly zero", func() {
			change, defined, index, latest := read(
				[]float64{6, 7, 9},
				[]bool{true, true, false},
				[]float64{5, 7, 1},
				[]bool{true, true, true},
			)
			So(change, ShouldResemble, []float64{1, 0, 0})
			So(defined, ShouldResemble, []bool{true, true, false})
			So(index, ShouldResemble, []int64{0, 1})
			So(latest, ShouldResemble, []float64{6, 7})

			Convey("And the next evaluation starts clean", func() {
				change, defined, index, _ := read([]float64{1}, []bool{false}, nil, nil)
				So(change, ShouldResemble, []float64{0})
				So(defined, ShouldResemble, []bool{false})
				So(index, ShouldBeEmpty)
			})
		})

		Convey("Nothing arriving is idle, with nothing to write back", func() {
			change, _, index, _ := read(nil, nil, nil, nil)
			So(change, ShouldBeEmpty)
			So(index, ShouldBeEmpty)
		})

		Convey("Mismatched presence is rejected", func() {
			So(client.Write(ctx, func(params calculus.Change_write_Params) error {
				_, err := params.NewValue(2)
				return err
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}
