package strategy

import (
	"bytes"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"testing"
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
}
