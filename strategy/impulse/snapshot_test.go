package impulse

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/tests/market"
)

func TestMarketSnapshot(t *testing.T) {
	Convey("An accepted visualization outlives subsequent producer updates", t, func() {
		impulseMap := NewMap()
		frames := market.ImpulseTape("BTC/USD", 4)
		So(impulseMap.Step(frames[0]), ShouldBeNil)
		held := impulseMap.Markets["BTC/USD"]
		snapshot := held.Snapshot().(*grid.Snapshot)
		original := snapshot.Cells[0]

		for _, frame := range frames[1:] {
			So(impulseMap.Step(frame), ShouldBeNil)
		}

		So(snapshot.Cells[0], ShouldResemble, original)
		So(snapshot.Sequence, ShouldEqual, 1)
		So(snapshot.Volume, ShouldEqual, "1.000000000000")
		So(held.Sequence, ShouldEqual, len(frames))
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
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		held.Snapshot()
	}
}
