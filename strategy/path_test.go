package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPathBookJSON(t *testing.T) {
	Convey("PathBook round-trips as absolute JSON", t, func() {
		book := NewPathBook(ActionHold)
		So(book.Action, ShouldEqual, "hold")
		So(book.Resistance, ShouldEqual, InitialPathResistance)
		So(book.StorageKey("R01/R02"), ShouldEqual, "model/paths/R01/R02/hold.bin")

		book.Apply(-0.05)
		decoded := DecodePathBook(book.Bytes())
		So(decoded, ShouldNotBeNil)
		So(decoded.Action, ShouldEqual, "hold")
		So(decoded.Resistance, ShouldAlmostEqual, 0.45, 1e-9)

		So(DecodePathBook(nil), ShouldBeNil)
		So(DecodePathBook([]byte(`{"noise":true}`)), ShouldBeNil)
	})
}
