package advisor

import (
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMomentum(t *testing.T) {
	Convey("Given the declarative Momentum Advisor", t, func() {
		momentum := NewMomentum()

		Convey("it exposes one Feature for each momentum phase", func() {
			So(momentum.Features, ShouldHaveLength, 4)
			So(momentum.Features[0].Class.Label, ShouldEqual, "Building")
			So(momentum.Features[1].Class.Label, ShouldEqual, "Sustaining")
			So(momentum.Features[2].Class.Label, ShouldEqual, "Stalling")
			So(momentum.Features[3].Class.Label, ShouldEqual, "Reversing")
		})

		Convey("every selector is qualified and unique inside its Feature", func() {
			for _, feature := range momentum.Features {
				So(feature.Clock, ShouldEqual, momentumClock)
				seen := make(map[string]bool, len(feature.Keys))

				for _, key := range feature.Keys {
					So(strings.Count(key, "/"), ShouldEqual, 1)
					So(seen[key], ShouldBeFalse)
					seen[key] = true
				}
			}
		})
	})
}
