package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestWireJSONObject(t *testing.T) {
	Convey("Given a typed gauge map from a signal producer", t, func() {
		source := map[string]any{
			"chart":    "gauge",
			"source":   "fluid",
			"category": types.CategoryLaminar,
			"samples":  4,
		}

		Convey("It should normalize through JSON for websocket fanout", func() {
			out, ok := wireJSONObject(source)

			So(ok, ShouldBeTrue)
			So(out["chart"], ShouldEqual, "gauge")
			So(out["source"], ShouldEqual, "fluid")
			So(out["category"], ShouldEqual, "laminar")
			So(out["samples"], ShouldEqual, 4)
		})
	})
}
