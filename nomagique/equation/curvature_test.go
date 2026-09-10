package equation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestNewCurvature(t *testing.T) {
	Convey("Curvature and prominence read neighbouring profile ordinates", t, func() {
		points := []equation.Point{{-1, 0.1}, {0, 0.9}, {1, 0.3}}
		curvature := tests.CollectSeq(equation.NewCurvature().Next(transport.Values(points...)))
		So(curvature[0], ShouldEqual, 1.4)
		prominence := tests.CollectSeq(equation.NewProminence().Next(transport.Values(points...)))
		So(prominence[0], ShouldEqual, 0.7)
	})
}
