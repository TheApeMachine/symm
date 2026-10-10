package ui

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCognitionTreeExportWire(t *testing.T) {
	Convey("Given an empty cognition tree", t, func() {
		Convey("the wire carries empty lists, not null", func() {
			wire, err := json.Marshal(CognitionTreeExport{}.normalized())
			So(err, ShouldBeNil)
			So(string(wire), ShouldEqual, `{"root":null,"branches":[],"feasible":[]}`)
		})
	})
}
