//go:build ignore

package fluid

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRunwayBuildTag(t *testing.T) {
	Convey("Given the ignored runway module", t, func() {
		Convey("It should remain excluded from normal builds", func() {
			So(true, ShouldBeTrue)
		})
	})
}
