package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSemantics(t *testing.T) {
	Convey("Semantics returns initialized maps", t, func() {
		semantics := Semantics()
		So(semantics.Metrics, ShouldNotBeNil)
		So(semantics.Signals, ShouldNotBeNil)
	})
}
