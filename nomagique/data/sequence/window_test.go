package sequence_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data/sequence"
)

func TestWindowWrite(t *testing.T) {
	ctx := context.Background()

	Convey("Given a window retaining three arrivals of two quantities", t, func() {
		client := sequence.Window_ServerToClient(sequence.NewWindow(ctx))
		defer client.Release()

		arrive := func(values []float64, present []bool) {
			err := client.Write(ctx, func(params sequence.Window_write_Params) error {
				params.SetSpan(3)

				held, err := params.NewValue(int32(len(values)))

				if err != nil {
					return err
				}

				for index, value := range values {
					held.Set(index, value)
				}

				carried, err := params.NewPresent(int32(len(present)))

				if err != nil {
					return err
				}

				for index, value := range present {
					carried.Set(index, value)
				}

				return nil
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
		}

		read := func() (out []float64, rows, cols int32, full bool) {
			future, release := client.Done(ctx, nil)
			defer release()

			results, err := future.Struct()
			So(err, ShouldBeNil)

			held, err := results.Out()
			So(err, ShouldBeNil)

			for index := range held.Len() {
				out = append(out, held.At(index))
			}

			return out, results.Rows(), results.Cols(), results.Full()
		}

		Convey("When fewer arrivals than the span have landed", func() {
			arrive([]float64{1, 2}, []bool{true, true})

			out, rows, cols, full := read()

			Convey("Then it holds what it has and says it is not full", func() {
				So(out, ShouldResemble, []float64{1, 2})
				So(rows, ShouldEqual, 1)
				So(cols, ShouldEqual, 2)
				So(full, ShouldBeFalse)
			})
		})

		Convey("When the span has been reached", func() {
			arrive([]float64{1, 2}, []bool{true, true})
			arrive([]float64{3, 4}, []bool{true, true})
			arrive([]float64{5, 6}, []bool{true, true})

			out, rows, cols, full := read()

			Convey("Then it hands back one matrix, newest row last", func() {
				So(out, ShouldResemble, []float64{1, 2, 3, 4, 5, 6})
				So(rows, ShouldEqual, 3)
				So(cols, ShouldEqual, 2)
				So(full, ShouldBeTrue)
			})
		})

		Convey("When more arrivals land than the span", func() {
			arrive([]float64{1, 2}, []bool{true, true})
			arrive([]float64{3, 4}, []bool{true, true})
			arrive([]float64{5, 6}, []bool{true, true})
			arrive([]float64{7, 8}, []bool{true, true})

			out, rows, _, _ := read()

			Convey("Then the oldest leaves as the newest arrives", func() {
				So(out, ShouldResemble, []float64{3, 4, 5, 6, 7, 8})
				So(rows, ShouldEqual, 3)
			})
		})

		Convey("When a quantity did not arrive", func() {
			arrive([]float64{1, 2}, []bool{true, true})
			arrive([]float64{3, 0}, []bool{true, false})

			out, rows, _, _ := read()

			// A zero standing in for a quantity that was not published would
			// assert it sat still, which is the opposite reading.
			Convey("Then the whole arrival is left out", func() {
				So(out, ShouldResemble, []float64{1, 2})
				So(rows, ShouldEqual, 1)
			})
		})
	})
}
