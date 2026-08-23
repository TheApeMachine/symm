package mcts

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestFrameToRow(t *testing.T) {
	Convey("Given a complete named reasoning frame", t, func() {
		frame := completeReasoningFrame()

		Convey("It should preserve the stable semantic column contract", func() {
			row, err := FrameToRow(frame)
			So(err, ShouldBeNil)
			So(row, ShouldHaveLength, ReasoningColumnCount)
			So(row[ColumnTreatment], ShouldEqual, ActionWait)
			So(row[ColumnMaximumHorizon], ShouldEqual, 3)

			restored, err := RowToFrame(row)
			So(err, ShouldBeNil)
			maximumHorizon, found := restored.Get(SymbolMaximumHorizon)
			So(found, ShouldBeTrue)
			So(maximumHorizon, ShouldEqual, 3)
		})
	})
}

func completeReasoningFrame() types.Frame {
	frame := types.Frame{}
	frame.Put(SymbolContextConfidence, 1)
	frame.Put(SymbolTreatment, ActionWait)
	frame.Put(SymbolTarget, 0)
	frame.Put(SymbolFlow, 1)
	frame.Put(SymbolLiquidityImpact, 0)
	frame.Put(SymbolHawkes, 0)
	frame.Put(SymbolCoherence, 1)
	frame.Put(SymbolRegime, 1)
	frame.Put(SymbolSurprise, 0)
	frame.Put(SymbolPosition, 0)
	frame.Put(SymbolHorizon, 0)
	frame.Put(SymbolMaximumHorizon, 3)
	frame.Put(SymbolArchetype, 0)
	frame.Put(SymbolVelocity, 0)
	frame.Put(SymbolStoredEnergy, 0)
	frame.Put(SymbolCausalExpectation, 0)
	frame.Put(SymbolSpread, 0)
	return frame
}
