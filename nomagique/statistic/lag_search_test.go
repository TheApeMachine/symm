package statistic

import (
	"context"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

/* lagFixture replays the original LEGACY 16-price, two-second lead fixture. */
func lagFixture(write func(int32) (store.PricePoint_List, error), shifted bool) error {
	points, err := write(16)

	if err != nil {
		return err
	}
	for index := range 16 {
		source := index

		if shifted {
			source = max(0, index-2)
		}
		points.At(index).SetAt(int64(index) * 1e9)
		points.At(index).SetValue(math.Exp(math.Sin(float64(source))))
	}
	return nil
}

func TestLagSearchWrite(t *testing.T) {
	Convey("Native lag search recovers observed seconds and clears unsupported inputs", t, func() {
		ctx := context.Background()
		client := LagSearch_ServerToClient(NewLagSearch())
		defer client.Release()
		for range 2 {
			err := client.Write(ctx, func(args LagSearch_write_Params) error {
				if err := lagFixture(args.NewLeft, false); err != nil {
					return err
				}
				return lagFixture(args.NewRight, true)
			})
			So(err, ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			result, err := future.Struct()
			So(err, ShouldBeNil)
			values, err := result.Values()
			So(err, ShouldBeNil)
			present, err := result.Present()
			So(err, ShouldBeNil)
			So(values.At(2), ShouldEqual, 2)
			So(values.At(3), ShouldEqual, 2)
			So(values.At(6), ShouldEqual, 14)
			So(values.At(7), ShouldEqual, 28)
			So(values.At(10), ShouldBeGreaterThan, 0)
			So(present.At(1), ShouldBeTrue)
			So(present.At(8), ShouldBeTrue)
			release()
		}
		Convey("No paths do not leave the previous result active", func() {
			So(client.Write(ctx, nil), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			present, err := result.Present()
			So(err, ShouldBeNil)
			for index := range present.Len() {
				So(present.At(index), ShouldBeFalse)
			}
		})
		Convey("Regressing timestamps fail before logarithms or overlap", func() {
			So(client.Write(ctx, func(args LagSearch_write_Params) error {
				points, err := args.NewLeft(2)
				if err != nil {
					return err
				}
				points.At(0).SetAt(2)
				points.At(0).SetValue(100)
				points.At(1).SetAt(1)
				points.At(1).SetValue(101)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}

func BenchmarkLagSearchWrite(b *testing.B) {
	ctx := context.Background()
	client := LagSearch_ServerToClient(NewLagSearch())
	defer client.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		err := client.Write(ctx, func(args LagSearch_write_Params) error {
			if err := lagFixture(args.NewLeft, false); err != nil {
				return err
			}
			return lagFixture(args.NewRight, true)
		})
		if err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		result, err := future.Struct()
		if err != nil {
			b.Fatal(err)
		}
		values, err := result.Values()
		if err != nil {
			b.Fatal(err)
		}
		if values.At(3) != 2 {
			b.Fatal("two-second lag lost")
		}
		release()
	}
}
