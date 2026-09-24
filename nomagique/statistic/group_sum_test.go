package statistic_test

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
)

func TestGroupSum(t *testing.T) {
	ctx := context.Background()

	Convey("Given values labelled into groups", t, func() {
		client := statistic.GroupSum_ServerToClient(statistic.NewGroupSum())
		defer client.Release()

		So(client.Write(ctx, func(params statistic.GroupSum_write_Params) error {
			values, err := params.NewValues(4)

			if err != nil {
				return err
			}

			present, err := params.NewPresent(4)

			if err != nil {
				return err
			}

			labels, err := params.NewLabels(4)

			if err != nil {
				return err
			}

			for element, entry := range []struct {
				value   float64
				present bool
				label   string
			}{{1, true, "x"}, {2, true, "y"}, {3, true, "x"}, {5, false, "z"}} {
				values.Set(element, entry.value)
				present.Set(element, entry.present)

				if err := labels.Set(element, entry.label); err != nil {
					return err
				}
			}

			return nil
		}), ShouldBeNil)
		So(client.WaitStreaming(), ShouldBeNil)

		future, release := client.Done(ctx, nil)
		defer release()

		results, err := future.Struct()
		So(err, ShouldBeNil)

		Convey("Present values are summed per label, and a group with none present is left out", func() {
			labels, err := results.Labels()
			So(err, ShouldBeNil)
			sums, err := results.Sums()
			So(err, ShouldBeNil)
			So(labels.Len(), ShouldEqual, 2)

			first, err := labels.At(0)
			So(err, ShouldBeNil)
			second, err := labels.At(1)
			So(err, ShouldBeNil)
			So(first, ShouldEqual, "x")
			So(second, ShouldEqual, "y")
			So(sums.At(0), ShouldEqual, 4)
			So(sums.At(1), ShouldEqual, 2)
		})
	})
}
