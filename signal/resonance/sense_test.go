package resonance

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestDefaultArchitecture(testingTB *testing.T) {
	Convey("Given the sensory channel contract", testingTB, func() {
		arch := DefaultArchitecture()

		So(len(arch), ShouldEqual, 3)
		So(arch[0], ShouldEqual, SensoryChannelCount)
		So(arch[1], ShouldEqual, SensoryChannelCount*2)
		So(arch[2], ShouldEqual, 3)
		So(validateArchitecture(arch), ShouldBeNil)
	})
}

func TestBuildSensoryVector(testingTB *testing.T) {
	Convey("Given a stubbed sensory vector builder", testingTB, func() {
		registry := newSenseRegistry()
		scope := "BTC/USD"

		vector, facts, ok := buildSensoryVector(scope, registry)

		Convey("It should withhold until tree-seek migration completes", func() {
			So(ok, ShouldBeFalse)
			So(vector, ShouldBeNil)
			So(facts.lastPrice, ShouldEqual, 0)
		})
	})
}

func TestMeasureTargets(testingTB *testing.T) {
	cases := []struct {
		category logic.CategoryType
		expected []string
	}{
		{
			category: logic.CategoryType(CategoryFlow),
			expected: []string{"fluid", "depthflow", "exhaust", "liquidity"},
		},
		{
			category: logic.CategoryType(CategoryStress),
			expected: []string{"toxicity", "hawkes", "pumpdump", "cvd"},
		},
		{
			category: logic.CategoryType(CategoryCoupling),
			expected: []string{
				"correlation", "leadlag", "causal", "sentiment", "manifold",
			},
		},
	}

	for _, testCase := range cases {
		Convey("Given resonance attention mode "+string(testCase.category), testingTB, func() {
			So(MeasureTargets(testCase.category), ShouldResemble, testCase.expected)
		})
	}
}
