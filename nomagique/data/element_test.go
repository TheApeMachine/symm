package data_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
elementAt drives the Element node over one list and position.
*/
func elementAt(t *testing.T, values []float64, index int32) (float64, bool) {
	t.Helper()
	ctx := context.Background()
	client := data.Element_ServerToClient(data.NewElement())

	err := client.Write(ctx, func(params data.Element_write_Params) error {
		list, err := params.NewValues(int32(len(values)))

		if err != nil {
			return err
		}

		for position, value := range values {
			list.Set(position, value)
		}

		params.SetIndex(index)
		return nil
	})

	if err != nil {
		t.Fatalf("element write: %v", err)
	}

	if err := client.WaitStreaming(); err != nil {
		t.Fatalf("element stream: %v", err)
	}

	future, release := client.Done(ctx, nil)
	defer release()
	results, err := future.Struct()

	if err != nil {
		t.Fatalf("element done: %v", err)
	}

	return results.Out(), results.Found()
}

func TestElementServer_Write(t *testing.T) {
	Convey("Given a list of values", t, func() {
		values := []float64{10, 0, -3.5}

		Convey("When a position inside the list is taken", func() {
			out, found := elementAt(t, values, 2)

			Convey("Then that value is reported as found", func() {
				So(out, ShouldEqual, -3.5)
				So(found, ShouldBeTrue)
			})
		})

		Convey("When the value at the position is zero", func() {
			out, found := elementAt(t, values, 1)

			Convey("Then it is still reported as found", func() {
				// This is the distinction the found flag exists for: a
				// genuine zero must not read the same as a missing one.
				So(out, ShouldEqual, 0)
				So(found, ShouldBeTrue)
			})
		})

		Convey("When the position is past the end of the list", func() {
			out, found := elementAt(t, values, 3)

			Convey("Then nothing is found rather than a zero invented", func() {
				So(out, ShouldEqual, 0)
				So(found, ShouldBeFalse)
			})
		})

		Convey("When the position is negative", func() {
			_, found := elementAt(t, values, -1)

			Convey("Then nothing is found and it does not count from the end", func() {
				So(found, ShouldBeFalse)
			})
		})

		Convey("When the list is empty", func() {
			_, found := elementAt(t, nil, 0)

			Convey("Then nothing is found", func() {
				So(found, ShouldBeFalse)
			})
		})
	})
}
