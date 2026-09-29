package impulse

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests/market"
)

func TestMarketSnapshot(t *testing.T) {
	Convey("An accepted visualization outlives subsequent producer updates", t, func() {
		impulseMap := NewMap()
		frames := market.ImpulseTape("BTC/USD", 4)
		So(impulseMap.Step(frames[0]), ShouldBeNil)
		held := impulseMap.Markets["BTC/USD"]
		snapshot := held.Snapshot()
		original := snapshot.Cells[0]

		for _, frame := range frames[1:] {
			So(impulseMap.Step(frame), ShouldBeNil)
		}

		So(snapshot.Cells[0], ShouldResemble, original)
		So(snapshot.Sequence, ShouldEqual, 1)
		So(snapshot.Volume, ShouldEqual, "1.000000000000")
		So(held.Sequence, ShouldEqual, len(frames))

		Convey("A defined zero SNR remains zero and does not fall back to Level²", func() {
			cell := held.Cells[0]
			cell.Baseline.Count = 10
			cell.Baseline.M2 = 1.0
			cell.Level = 2.0
			cell.owner.measurement.SNR = 0.0
			cell.owner.measurement.SNRDefined = true

			snap := held.Snapshot()
			So(snap.Cells[0].Quality, ShouldEqual, 0.0)

			cell.owner.measurement.SNRDefined = false
			snap = held.Snapshot()
			So(snap.Cells[0].Quality, ShouldEqual, 4.0)
		})
	})
}

func BenchmarkMarketSnapshot(b *testing.B) {
	impulseMap := NewMap()

	for _, frame := range market.ImpulseTape("BTC/USD", 4) {
		if err := impulseMap.Step(frame); err != nil {
			b.Fatal(err)
		}
	}

	held := impulseMap.Markets["BTC/USD"]
	b.ReportAllocs()

	for b.Loop() {
		held.Snapshot()
	}
}
