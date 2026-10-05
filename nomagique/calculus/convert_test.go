package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestConvert(t *testing.T) {
	Convey("Convert passes adapter through without error", t, func() {
		op := NewConvert()
		state := data.NewState(data.NewMap("value", "value"))
		adapter := data.NewAdapter(nil, state)

		result := data.Read[*data.Adapter](op.Next(data.NewValue(adapter)))
		So(op.Error(), ShouldBeNil)
		So(result, ShouldEqual, adapter)
	})
}
