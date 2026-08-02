package level3

import (
	"testing"

	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
)

func TestNewDecoderFixture(t *testing.T) {
	Convey("Given the level3 fixture package", t, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewDecoderFixture(SNAPSHOT, 1)

			Convey("Then it should emit one level3 snapshot without order events", func() {
				var frame map[string]any
				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
				}

				row := frame["data"].([]any)[0].(map[string]any)
				bid := row["bids"].([]any)[0].(map[string]any)

				So(frame["channel"], ShouldEqual, "level3")
				So(frame["type"], ShouldEqual, "snapshot")
				So(row["bids"], ShouldNotBeEmpty)
				So(bid["event"], ShouldBeNil)
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewDecoderFixture(UPDATE, 3)

			Convey("Then it should generate an ordered level3 event sequence", func() {
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					row := frame["data"].([]any)[0].(map[string]any)
					ask := row["asks"].([]any)[0].(map[string]any)

					So(frame["type"], ShouldEqual, "update")
					So(ask["event"], ShouldEqual, "delete")
					So(uint64(row["checksum"].(float64)), ShouldBeGreaterThan, 0)
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})
	})
}

func TestInject(t *testing.T) {
	Convey("Given a Level3 fixture instance", t, func() {
		fixture := NewDecoderFixture(SNAPSHOT, 1)
		row := map[string]any{}
		now := time.Now().UTC()

		Convey("When injecting orders with different priorities", func() {
			orders := []tests.Order{
				{ID: "order-2", Price: 100.0, Qty: 1.5, Priority: 2, At: now},
				{ID: "order-1", Price: 100.0, Qty: 2.0, Priority: 1, At: now},
			}

			fixture.inject(row, "bids", orders)
			bids := row["bids"].([]any)

			Convey("Then orders should be sorted by priority at the same price", func() {
				So(len(bids), ShouldEqual, 2)
				first := bids[0].(map[string]any)
				second := bids[1].(map[string]any)
				So(first["order_id"], ShouldEqual, "order-1")
				So(second["order_id"], ShouldEqual, "order-2")
			})
		})
	})
}

/*
BenchmarkFixture_Generate measures exact-decimal Level3 transition rendering.
*/
func BenchmarkFixture_Generate(b *testing.B) {
	generator := tests.NewGenerator("SIM1/USD", 100.0, 42)
	fixture := NewMarket([]string{"SIM1/USD"}, generator)
	b.ReportAllocs()

	for b.Loop() {
		generator.SetState(tests.Baseline)

		for payload := range fixture.Generate() {
			if len(payload) == 0 {
				b.Fatal("empty Level3 payload")
			}
		}
	}
}
