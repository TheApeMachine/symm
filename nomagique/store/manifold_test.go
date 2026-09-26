package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
)

/*
	manifoldFixture exercises the native RPC and actual GPU at eight cells per

axis, a test resource resolution rather than a market calibration parameter.
*/
type manifoldFixture struct {
	client   Manifold
	sequence int64
	grid     uint32
}

func (fixture *manifoldFixture) write(symbol string, orders bool, offset int, buy float64) error {
	fixture.sequence++
	return fixture.client.Write(context.Background(), func(args Manifold_write_Params) error {
		args.SetEpoch(1)
		args.SetSequence(fixture.sequence)
		dimension := fixture.grid
		if dimension == 0 {
			dimension = 8
		}
		args.SetGridX(dimension)
		args.SetGridY(dimension)
		args.SetGridZ(dimension)
		if err := args.SetSymbol(symbol); err != nil {
			return err
		}
		excitation, err := args.NewExcitation(2)
		if err != nil {
			return err
		}
		present, err := args.NewPresent(2)
		if err != nil {
			return err
		}
		excitation.Set(0, buy)
		excitation.Set(1, 0)
		present.Set(0, true)
		present.Set(1, true)
		if !orders {
			return nil
		}
		market, err := args.NewMarket()
		if err != nil {
			return err
		}
		if err := market.SetSymbol(symbol); err != nil {
			return err
		}
		market.SetUpdated(true)
		entries, err := market.NewOrders(4)
		if err != nil {
			return err
		}
		for index := range 4 {
			entry := entries.At(index)
			entry.SetBid(index < 2)
			entry.SetRank(uint32(index % 2))
			if err := entry.SetId(fmt.Sprintf("order-%t-%d", index < 2, index+offset)); err != nil {
				return err
			}
			if err := entry.SetPrice(fmt.Sprintf("%d", 99+index)); err != nil {
				return err
			}
			if err := entry.SetQuantity(fmt.Sprintf("%d", index+1)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (fixture *manifoldFixture) await(t testing.TB) ManifoldFrame {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		future, release := fixture.client.Done(context.Background(), nil)
		result, err := future.Struct()
		if err != nil {
			release()
			t.Fatal(err)
		}
		frame, err := result.Frame()
		if err != nil {
			release()
			t.Fatal(err)
		}
		if frame.IsValid() && frame.Sequence() == fixture.sequence {
			message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
			if err != nil {
				release()
				t.Fatal(err)
			}
			owned, err := NewRootManifoldFrame(segment)
			if err != nil {
				release()
				message.Release()
				t.Fatal(err)
			}
			if err := capnp.Struct(owned).CopyFrom(capnp.Struct(frame)); err != nil {
				release()
				message.Release()
				t.Fatal(err)
			}
			release()
			t.Logf("physical frame sequence=%d population=%d substeps=%d time=%g", owned.Sequence(), owned.Population(), owned.Substeps(), owned.PhysicalTime())
			return owned
		}
		release()
		time.Sleep(time.Millisecond)
	}
	t.Fatal("native physical frame did not reach requested sequence")
	return ManifoldFrame{}
}

func TestManifoldWrite(t *testing.T) {
	Convey("The native retained manifold consumes complete per-order venue queues", t, func() {
		server := NewManifold()
		fixture := &manifoldFixture{client: Manifold_ServerToClient(server)}
		defer fixture.client.Release()
		So(fixture.write("BTC/USD", true, 0, 0), ShouldBeNil)
		first := fixture.await(t)
		defer first.Message().Release()
		So(first.Population(), ShouldEqual, 4)
		So(first.AcceptedStep(), ShouldBeGreaterThan, 0)
		So(first.PhysicalTime(), ShouldBeGreaterThan, 0)
		So(fixture.write("ETH/USD", true, 0, 0), ShouldBeNil)
		second := fixture.await(t)
		defer second.Message().Release()
		So(second.Population(), ShouldEqual, 8)
		So(fixture.write("BTC/USD", true, 2, 0), ShouldBeNil)
		third := fixture.await(t)
		defer third.Message().Release()
		So(third.Population(), ShouldEqual, 8)
		So(fixture.write("BTC/USD", false, 0, 1), ShouldBeNil)
		forced := fixture.await(t)
		defer forced.Message().Release()
		So(forced.Population(), ShouldEqual, 8)
		So(forced.PhysicalTime(), ShouldBeGreaterThan, third.PhysicalTime())
		So(fixture.write("BTC/USD", false, 0, 1), ShouldBeNil)
		future, release := fixture.client.Done(context.Background(), nil)
		defer release()
		result, err := future.Struct()
		So(err, ShouldBeNil)
		So(result.Sequence(), ShouldEqual, 4)
		So(result.HasFrame(), ShouldBeFalse)
		present, err := result.Present()
		So(err, ShouldBeNil)
		for index := range present.Len() {
			So(present.At(index), ShouldBeTrue)
		}
		So(first.Sequence(), ShouldEqual, 1)
		So(first.Population(), ShouldEqual, 4)
	})
}

func BenchmarkManifoldWrite(b *testing.B) {
	for _, dimension := range []uint32{8, 64} {
		b.Run(fmt.Sprintf("grid%d", dimension), func(b *testing.B) {
			fixture := &manifoldFixture{client: Manifold_ServerToClient(NewManifold()), grid: dimension}
			defer fixture.client.Release()
			if err := fixture.write("BTC/USD", true, 0, 0); err != nil {
				b.Fatal(err)
			}
			frame := fixture.await(b)
			frame.Message().Release()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				if err := fixture.write("BTC/USD", true, index%2+1, float64(index%2)); err != nil {
					b.Fatal(err)
				}
				frame := fixture.await(b)
				frame.Message().Release()
			}
		})
	}
}
