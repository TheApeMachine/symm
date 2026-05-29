package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestNewPumpPerspective(t *testing.T) {
	Convey("Given a pump perspective", t, func() {
		perspective := NewPumpPerspective()

		Convey("It should attach the pump tree playbook", func() {
			So(perspective.Type, ShouldEqual, PerspectivePump)
			So(perspective.Tree, ShouldNotBeNil)
			So(perspective.Tree.Branch, ShouldNotBeNil)
		})
	})
}

func TestPerspectiveIngestAndHasCategory(t *testing.T) {
	Convey("Given a pump perspective with measurements", t, func() {
		perspective := NewPumpPerspective()

		err := perspective.Ingest(engine.Measurement{
			Source:     "pumpdump",
			Category:   engine.CatCoiledCompression,
			Confidence: 0.8,
		})

		Convey("It should recognize coiled compression", func() {
			So(err, ShouldBeNil)
			So(perspective.HasCategory(perspectives.CategoryCoiledCompression), ShouldBeTrue)
			So(perspective.HasCategory(perspectives.CategoryVerticalIgnition), ShouldBeFalse)
		})
	})
}
