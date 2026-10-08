package correlation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/correlation"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestRelativeNext(t *testing.T) {
	Convey("Relative tracks baseline, divergence, and zscore of relative energy", t, func() {
		node := correlation.NewRelative()

		var outInitial []float64
		for val := range node.Next(data.NewValue(1.0).Next(nil)) {
			outInitial = append(outInitial, *(*float64)(val))
		}
		So(node.Error(), ShouldBeNil)
		So(len(outInitial), ShouldEqual, 4)
		So(outInitial[3], ShouldEqual, 1.0)

		nodeZero := correlation.NewRelative()
		var outZero []float64
		for val := range nodeZero.Next(data.NewValue(0.0).Next(nil)) {
			outZero = append(outZero, *(*float64)(val))
		}
		So(nodeZero.Error(), ShouldBeNil)
		So(len(outZero), ShouldEqual, 4)
		So(outZero[3], ShouldEqual, 0.0)
	})
}
