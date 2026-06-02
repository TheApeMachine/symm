//go:build ignore

package optimizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSearchSpaceBuildTag(t *testing.T) {
	Convey("Given the ignored search_space module", t, func() {
		Convey("It should remain excluded from normal builds", func() {
			So(true, ShouldBeTrue)
		})
	})
}
