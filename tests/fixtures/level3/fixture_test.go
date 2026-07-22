package level3

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	marketsignal "github.com/theapemachine/symm/tests/fixtures/signal"
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

/*
BenchmarkFixture_Generate measures exact-decimal Level3 transition rendering.
*/
func BenchmarkFixture_Generate(b *testing.B) {
	signal := marketsignal.New([]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"})
	fixture := NewMarket([]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}, signal)
	b.ReportAllocs()

	for b.Loop() {
		if err := signal.Transition(marketsignal.Baseline); err != nil {
			b.Fatal(err)
		}

		for payload := range fixture.Generate() {
			if len(payload) == 0 {
				b.Fatal("empty Level3 payload")
			}
		}
	}
}
