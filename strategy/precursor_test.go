package strategy

import (
	"bytes"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

func TestPrecursorEncode(t *testing.T) {
	Convey("Market and holding state remain part of the context address", t, func() {
		precursor := NewPrecursor()
		impulse := &grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 42}}}
		flat := bytes.Clone(precursor.Encode(impulse, false))
		held := bytes.Clone(precursor.Encode(impulse, true))
		So(bytes.Equal(flat, held), ShouldBeFalse)
		impulse.Label = "ETH/USD"
		So(bytes.Equal(flat, precursor.Encode(impulse, false)), ShouldBeFalse)
		impulse.Ready = false
		So(precursor.Encode(impulse, false), ShouldBeNil)
	})

	Convey("Next dynamic holding state updates context address", t, func() {
		precursor := NewPrecursor()
		isHeld := false
		precursor.SetHolding(func(symbol string) bool {
			return isHeld
		})

		impulse := &grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 42}}}
		input := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(impulse))
		}

		for item := range precursor.Next(input) {
			command := (*cognition.Command)(item)
			So(command.Evaluate, ShouldNotBeNil)
			So(bytes.Equal(command.Evaluate.Context, precursor.Encode(impulse, false)), ShouldBeTrue)
		}

		isHeld = true
		for item := range precursor.Next(input) {
			command := (*cognition.Command)(item)
			So(command.Evaluate, ShouldNotBeNil)
			So(bytes.Equal(command.Evaluate.Context, precursor.Encode(impulse, true)), ShouldBeTrue)
		}
	})
}
