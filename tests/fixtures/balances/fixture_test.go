package balances

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the balances fixture package", t, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewFixture(SNAPSHOT, 1)

			Convey("Then it should emit one balance snapshot frame", func() {
				var frame map[string]any
				count := 0

				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					count++
				}

				So(count, ShouldEqual, 1)
				So(frame["channel"], ShouldEqual, "balances")
				So(frame["type"], ShouldEqual, "snapshot")
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)
			identical := NewFixture(UPDATE, 3)

			Convey("Then it should generate balance update frames", func() {
				sequence := float64(-1)
				count := 0
				payloads := [][]byte{}

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					So(frame["channel"], ShouldEqual, "balances")
					So(frame["type"], ShouldEqual, "update")
					So(frame["sequence"].(float64), ShouldBeGreaterThan, sequence)
					sequence = frame["sequence"].(float64)
					payloads = append(payloads, payload)
					count++
				}
				identicalPayloads := [][]byte{}

				for payload := range identical.Generate() {
					identicalPayloads = append(identicalPayloads, payload)
				}

				So(count, ShouldEqual, 3)
				So(payloads, ShouldResemble, identicalPayloads)
			})
		})

		Convey("When an invalid update source is requested", func() {
			So(func() { NewFixture(UPDATE, 0) }, ShouldPanic)
			So(func() {
				(&Fixture{horizon: 1}).sequencer([]byte(
					`{"channel":"balances","type":"update","sequence":1,` +
						`"data":[{"balance":1,"timestamp":"invalid"}]}`,
				))
			}, ShouldPanic)
		})
	})
}

/*
BenchmarkNewFixture measures deterministic balance update materialization.
*/
func BenchmarkNewFixture(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		fixture := NewFixture(UPDATE, 16)

		if len(fixture.sequence) != 16 {
			b.Fatal("balance fixture sequence incomplete")
		}
	}
}
