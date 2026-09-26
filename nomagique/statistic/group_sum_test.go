package statistic_test

import (
	"context"
	strconv "strconv"
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

/* TestGroupSumWrite distinguishes absent observations from malformed observations. */
func TestGroupSumWrite(t *testing.T) {
	Convey("Configured regions alone do not create an activation", t, func() {
		client := statistic.GroupSum_ServerToClient(statistic.NewGroupSum())
		defer client.Release()
		for _, present := range []bool{true, false, true} {
			So(client.Write(context.Background(), func(params statistic.GroupSum_write_Params) error {
				labels, err := params.NewLabels(1)
				if err != nil {
					return err
				}
				if err := labels.Set(0, "region"); err != nil {
					return err
				}
				if !present {
					return nil
				}
				values, err := params.NewValues(1)
				if err != nil {
					return err
				}
				values.Set(0, 7)
				known, err := params.NewPresent(1)
				if err != nil {
					return err
				}
				known.Set(0, true)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(context.Background(), nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			values, err := result.Sums()
			So(err, ShouldBeNil)
			if !present {
				So(values.Len(), ShouldEqual, 0)
			}
			if present {
				So(values.Len(), ShouldEqual, 1)
				So(values.At(0), ShouldEqual, 7)
			}
			release()
		}
	})
}

func BenchmarkGroupSumWrite(b *testing.B) {
	client := statistic.GroupSum_ServerToClient(statistic.NewGroupSum())
	defer client.Release()
	b.ReportAllocs()
	for b.Loop() {
		if err := client.Write(context.Background(), func(params statistic.GroupSum_write_Params) error {
			labels, err := params.NewLabels(411)
			if err != nil {
				return err
			}
			values, err := params.NewValues(411)
			if err != nil {
				return err
			}
			present, err := params.NewPresent(411)
			if err != nil {
				return err
			}
			for index := range 411 {
				if err := labels.Set(index, strconv.Itoa(index%3)); err != nil {
					return err
				}
				values.Set(index, float64(index))
				present.Set(index, true)
			}
			return nil
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(context.Background(), nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
