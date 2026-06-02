package depthflow

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestDepthImbalanceRatio(t *testing.T) {
	Convey("Given book depth levels from config", t, func() {
		testconfig.Load(t)

		symbol, err := NewDepthSymbol("ETH/EUR")
		So(err, ShouldBeNil)

		Convey("It should construct a depth symbol", func() {
			So(symbol, ShouldNotBeNil)
		})
	})
}

