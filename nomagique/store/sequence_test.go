package store

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSequenceWrite(t *testing.T) {
	Convey("Given an explicit append-only sequence", t, func() {
		ctx := context.Background()
		client := Sequence_ServerToClient(NewSequence())
		defer client.Release()
		step := func(index uint64, values ...string) (string, uint64, bool) {
			So(client.Write(ctx, func(args Sequence_write_Params) error {
				args.SetIndex(index)
				arrivals, err := args.NewAppend(int32(len(values)))
				if err != nil {
					return err
				}
				for position, value := range values {
					if err := arrivals.Set(position, []byte(value)); err != nil {
						return err
					}
				}
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			if !result.Found() {
				So(result.Which(), ShouldEqual, Item_Which_missing)
				return "", result.Count(), false
			}
			So(result.Which(), ShouldEqual, Item_Which_item)
			So(result.Item().Next() == index+1, ShouldBeTrue)
			payload, err := result.Item().Out()
			So(err, ShouldBeNil)
			return string(payload), result.Count(), true
		}
		value, count, found := step(0, "first", "", "second")
		So(value, ShouldEqual, "first")
		So(count, ShouldEqual, 2)
		So(found, ShouldBeTrue)
		value, count, found = step(1, "third")
		So(value, ShouldEqual, "second")
		So(count, ShouldEqual, 3)
		So(found, ShouldBeTrue)
		value, _, found = step(0)
		So(value, ShouldEqual, "first")
		So(found, ShouldBeTrue)
		value, _, found = step(2)
		So(value, ShouldEqual, "third")
		So(found, ShouldBeTrue)
		_, count, found = step(^uint64(0))
		So(count, ShouldEqual, 3)
		So(found, ShouldBeFalse)
		value, _, found = step(1)
		So(value, ShouldEqual, "second")
		So(found, ShouldBeTrue)
	})
}

func BenchmarkSequenceWrite(b *testing.B) {
	ctx := context.Background()
	client := Sequence_ServerToClient(NewSequence())
	defer client.Release()
	payload := []byte(`{"capture":{"session":"fixture","endpoint":"spot"},"cursor":{"sequence":9007199254740993,"record":1},"market":{"channel":"ticker","data":{"symbol":"BTC/USD","last":100}}}`)
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		if err := client.Write(ctx, func(args Sequence_write_Params) error {
			args.SetIndex(uint64(index))
			arrivals, err := args.NewAppend(1)
			if err != nil {
				return err
			}
			return arrivals.Set(0, payload)
		}); err != nil {
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
		if !result.Found() {
			b.Fatal("missing appended record")
		}
		release()
	}
}
