package strategy

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
)

func TestPrecursorEncode(t *testing.T) {
	Convey("Market and ordered regions define the context address", t, func() {
		precursor := NewPrecursor()
		impulse := &grid.Impulse{Label: "BTC/USD", Ready: true, Regions: []grid.Region{{Condition: 42}}}
		flat := bytes.Clone(precursor.Encode(impulse))
		impulse.Label = "ETH/USD"
		So(bytes.Equal(flat, precursor.Encode(impulse)), ShouldBeFalse)
		impulse.Label = "BTC/USD"
		impulse.Regions[0].Condition++
		So(bytes.Equal(flat, precursor.Encode(impulse)), ShouldBeFalse)
		impulse.Ready = false
		So(precursor.Encode(impulse), ShouldBeNil)
	})
}
