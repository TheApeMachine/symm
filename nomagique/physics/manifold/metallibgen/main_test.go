package main

import (
	"slices"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGeneratorMetalArgs(t *testing.T) {
	Convey("Given the shared Metal library generator", t, func() {
		generator := NewGenerator("/source", "/temporary")
		generator.metalStd = "metal4.1"

		Convey("It should preserve strict floating-point semantics for both physics kernels", func() {
			for _, name := range []string{"fluid.metal", "manifold.metal"} {
				arguments := generator.MetalArgs(name)
				So(slices.Contains(arguments, "-ffp-contract=off"), ShouldBeTrue)
				So(slices.Contains(arguments, "-fno-fast-math"), ShouldBeTrue)
			}
		})

		Convey("It should leave ordinary Metal sources on compiler defaults", func() {
			arguments := generator.MetalArgs("visualization.metal")
			So(slices.Contains(arguments, "-ffp-contract=off"), ShouldBeFalse)
			So(slices.Contains(arguments, "-fno-fast-math"), ShouldBeFalse)
		})
	})
}
