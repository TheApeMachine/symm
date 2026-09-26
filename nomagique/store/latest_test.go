package store

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/* TestLatestWrite exercises replacements, causal ordering and epoch changes. */
func TestLatestWrite(t *testing.T) {
	Convey("A native latest-value node retains each cohort member independently", t, func() {
		client := Latest_ServerToClient(NewLatest())
		defer client.Release()
		ctx := context.Background()
		write := func(key string, value float64, epoch, sequence int64) error {
			if err := client.Write(ctx, func(args Latest_write_Params) error {
				args.SetValue(value)
				args.SetEpoch(epoch)
				args.SetSequence(sequence)
				return args.SetKey(key)
			}); err != nil {
				return err
			}
			return client.WaitStreaming()
		}
		So(write("BTC/USD", 1, 7, 0), ShouldBeNil)
		So(write("ETH/USD", -2, 7, 1), ShouldBeNil)
		So(write("BTC/USD", 3, 7, 2), ShouldBeNil)

		Convey("The complete cohort preserves each member's own last-update stamp", func() {
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			values, err := result.Values()
			So(err, ShouldBeNil)
			sequences, err := result.Sequences()
			So(err, ShouldBeNil)
			keys, err := result.Keys()
			So(err, ShouldBeNil)
			So(keys.Len(), ShouldEqual, 2)
			So([]float64{values.At(0), values.At(1)}, ShouldResemble, []float64{3, -2})
			So([]int64{sequences.At(0), sequences.At(1)}, ShouldResemble, []int64{2, 1})
			So(result.Epoch(), ShouldEqual, 7)
		})

		Convey("A new epoch clears the old cohort", func() {
			So(write("SOL/USD", 4, 8, 0), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			values, err := result.Values()
			So(err, ShouldBeNil)
			So([]float64{values.At(0)}, ShouldResemble, []float64{4})
		})

		Convey("A regressing observation fails instead of replacing current evidence", func() {
			So(write("BTC/USD", 999, 7, 1), ShouldNotBeNil)
		})
	})
}

/* BenchmarkLatestWrite reports full-cohort publication after one member update. */
func BenchmarkLatestWrite(b *testing.B) {
	client := Latest_ServerToClient(NewLatest())
	defer client.Release()
	ctx := context.Background()
	// A 1,001-market fixture deliberately exceeds the former 600-market cap.
	keys := make([]string, 1001)
	for index := range keys {
		keys[index] = fmt.Sprintf("market-%d/USD", index)
	}
	write := func(sequence int) {
		if err := client.Write(ctx, func(args Latest_write_Params) error {
			args.SetEpoch(1)
			args.SetSequence(int64(sequence))
			args.SetValue(float64(sequence))
			return args.SetKey(keys[sequence%len(keys)])
		}); err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
	}
	for index := range keys {
		write(index)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		write(len(keys) + index)
		future, release := client.Done(ctx, nil)
		_, err := future.Struct()
		release()
		if err != nil {
			b.Fatal(err)
		}
	}
}
