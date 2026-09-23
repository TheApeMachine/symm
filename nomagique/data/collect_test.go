package data

import (
	"context"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestCollectWrite(t *testing.T) {
	Convey("Given a Cap'n Proto collection flushed by graph boundaries", t, func() {
		ctx := context.Background()
		client := Collect_ServerToClient(NewCollect())
		defer client.Release()
		step := func(values []string, flush bool) string {
			So(client.Write(ctx, func(params Collect_write_Params) error {
				params.SetFlush(flush)
				list, err := params.NewData(int32(len(values)))

				if err != nil {
					return err
				}
				for index, value := range values {
					if err := list.Set(index, []byte(value)); err != nil {
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

			if result.Which() == Collected_Which_idle {
				return ""
			}
			payload, err := result.Out()
			So(err, ShouldBeNil)
			return string(payload)
		}
		Convey("It retains earlier values, includes the final arrival, and resets between collections", func() {
			So(step([]string{`"BTC/USD"`, `9007199254740993`}, false), ShouldEqual, "")
			So(step([]string{`"ETH/USD"`}, true), ShouldEqual, `["BTC/USD",9007199254740993,"ETH/USD"]`)
			So(step(nil, true), ShouldEqual, "")
			So(step([]string{`{"nested":[1,true]}`}, true), ShouldEqual, `[{"nested":[1,true]}]`)
		})
		Convey("A flush without a current value still emits the prior collection", func() {
			So(step([]string{`"BTC/USD"`}, false), ShouldEqual, "")
			So(step(nil, true), ShouldEqual, `["BTC/USD"]`)
		})
		Convey("Malformed input fails the capability", func() {
			So(client.Write(ctx, func(params Collect_write_Params) error {
				list, err := params.NewData(1)

				if err != nil {
					return err
				}
				return list.Set(0, []byte(`{"broken":`))
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}

func BenchmarkCollectWrite(b *testing.B) {
	ctx := context.Background()
	client := Collect_ServerToClient(NewCollect())
	defer client.Release()
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		if err := client.Write(ctx, func(params Collect_write_Params) error {
			// Fixture: a discovery collection with 1,000 complete symbol values.
			list, err := params.NewData(1000)

			if err != nil {
				return err
			}
			for slot := range list.Len() {
				if err := list.Set(slot, []byte(`"BTC/USD"`)); err != nil {
					return err
				}
			}
			params.SetFlush(true)
			return nil
		}); err != nil {
			b.Fatal(err)
		}

		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err := future.Struct()
		release()

		if err != nil {
			b.Fatal(err)
		}
	}
}
