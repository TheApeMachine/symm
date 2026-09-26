package store

import (
	"context"
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRelationsWrite(t *testing.T) {
	Convey("A native shared relation store joins asynchronous spot markets", t, func() {
		ctx := context.Background()
		client := Relations_ServerToClient(NewRelations())
		defer client.Release()
		sequence := int64(0)
		var frozen Relations_done_Results
		var releaseFrozen func()
		for index := range 16 {
			for market, key := range []string{"BTC/USD", "ETH/USD", "SOL/USD"} {
				sequence++
				err := client.Write(ctx, func(args Relations_write_Params) error {
					if err := args.SetKey(key); err != nil {
						return err
					}
					args.SetPrice(math.Exp(math.Sin(float64(index) + []float64{0, 1.8, 2.1}[market])))
					args.SetTimestamp(float64(index)*1e9 + float64(market)*1e8)
					args.SetEpoch(1)
					args.SetSequence(sequence)
					return nil
				})
				So(err, ShouldBeNil)
				So(client.WaitStreaming(), ShouldBeNil)
				future, release := client.Done(ctx, nil)
				result, err := future.Struct()
				So(err, ShouldBeNil)
				if index == 8 && market == 0 {
					frozen = result
					releaseFrozen = release
					continue
				}
				if index == 15 && market == 0 {
					values, err := result.Values()
					So(err, ShouldBeNil)
					present, err := result.Present()
					So(err, ShouldBeNil)
					peer, err := result.Peer()
					So(err, ShouldBeNil)
					So(peer, ShouldEqual, "SOL/USD")
					So(present.At(0), ShouldBeTrue)
					So(values.At(2), ShouldBeGreaterThan, 1)
					So(values.At(14), ShouldEqual, 2)
					So(values.At(15), ShouldBeGreaterThan, 1)
					So(result.Sequence(), ShouldEqual, 46)
					So(result.PeerSequence(), ShouldEqual, 45)
				}
				release()
			}
		}
		frozenLeft, err := frozen.Left()
		So(err, ShouldBeNil)
		So(frozen.Sequence(), ShouldEqual, 25)
		So(frozenLeft.At(frozenLeft.Len()-1).At(), ShouldEqual, 8e9)
		releaseFrozen()
		Convey("A new epoch clears peer evidence", func() {
			So(client.Write(ctx, func(args Relations_write_Params) error {
				if err := args.SetKey("BTC/USD"); err != nil {
					return err
				}
				args.SetPrice(100)
				args.SetTimestamp(20e9)
				args.SetEpoch(2)
				args.SetSequence(0)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldBeNil)
			future, release := client.Done(ctx, nil)
			defer release()
			result, err := future.Struct()
			So(err, ShouldBeNil)
			present, err := result.Present()
			So(err, ShouldBeNil)
			So(present.At(0), ShouldBeFalse)
		})
		Convey("Regressing source stamps are explicit errors", func() {
			So(client.Write(ctx, func(args Relations_write_Params) error {
				if err := args.SetKey("BTC/USD"); err != nil {
					return err
				}
				args.SetPrice(100)
				args.SetTimestamp(20e9)
				args.SetEpoch(1)
				args.SetSequence(0)
				return nil
			}), ShouldBeNil)
			So(client.WaitStreaming(), ShouldNotBeNil)
		})
	})
}

func BenchmarkRelationsWrite(b *testing.B) {
	// 600 markets and 32 retained observed prices exercise full-cohort work.
	server := NewRelations()
	for market := range 600 {
		key := fmt.Sprintf("MARKET%03d/USD", market)
		path := &priceHistory{}
		for index := range 32 {
			if err := path.append(int64(index)*1e9, math.Exp(math.Sin(float64(index)+float64(market)/600))); err != nil {
				b.Fatal(err)
			}
		}
		path.measured.Measure()
		server.paths[key] = path
		server.keys = append(server.keys, key)
	}
	ctx := context.Background()
	client := Relations_ServerToClient(server)
	defer client.Release()
	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		err := client.Write(ctx, func(args Relations_write_Params) error {
			if err := args.SetKey(fmt.Sprintf("MARKET%03d/USD", index%600)); err != nil {
				return err
			}
			args.SetPrice(math.Exp(math.Sin(float64(index + 32))))
			args.SetTimestamp(float64(index+32) * 1e9)
			args.SetEpoch(0)
			args.SetSequence(int64(index))
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
		if err := client.WaitStreaming(); err != nil {
			b.Fatal(err)
		}
		future, release := client.Done(ctx, nil)
		_, err = future.Struct()
		if err != nil {
			b.Fatal(err)
		}
		release()
	}
}
