package equation_test

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/algo"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestNewLagProfile(t *testing.T) {
	Convey("Given nanosecond lag coordinates that do not round-trip through seconds", t, func() {
		for _, spacing := range []float64{129001, 856000, 129001.5} {
			left := tests.Path([]int64{0, 1105000, 2181000, 3037000}, []float64{1, 2, 1.5, 3})
			right := tests.Path([]int64{900000, 1800000, 2700000, 3600000}, []float64{1, 1.5, 2, 1.8})
			node := equation.NewLagProfile(algo.NewHayashiYoshida(),
				transport.NewIO(core.From(spacing)), transport.NewIO(core.From(5.0)))
			profile := tests.Drain(t, node, tests.Observation(left, right))
			So(node.Error(), ShouldBeNil)
			So(len(profile), ShouldEqual, 11)

			Convey(fmt.Sprintf("Spacing %g preserves each discrete position", spacing), func() {
				for index, candidate := range profile {
					fields := tests.Fields(t, candidate)
					So(fields, ShouldContainKey, "index")
					So(fields, ShouldContainKey, "lag_index")
					So(core.To[float64](fields["index"]), ShouldEqual, index)
					So(core.To[float64](fields["lag_index"]), ShouldEqual, index-5)
				}
			})
		}
	})

	times := []int64{0, 1e9, 2e9, 3e9, 4e9, 5e9}
	left := tests.Path(times, []float64{1, 2, 1.5, 3, 2.2, 4})
	right := tests.Path([]int64{1e9, 2e9, 3e9, 4e9, 5e9, 6e9}, []float64{1, 2, 1.5, 3, 2.2, 4})
	node := equation.NewLagProfile(algo.NewHayashiYoshida(), transport.NewIO(core.From(1e9)), transport.NewIO(core.From(2.0)))
	profile := tests.Drain(t, node, tests.Observation(left, right))
	if node.Error() != nil {
		t.Fatal(node.Error())
	}
	if len(profile) != 5 {
		t.Fatal(len(profile))
	}
	points := []core.Primitive{}
	for _, v := range profile {
		m := v.(map[string]core.Primitive)
		if _, ok := m["support"]; !ok {
			t.Fatal("candidate support discarded")
		}
		points = append(points, core.From(m))
	}
	peak := tests.Drain(t, equation.NewPeak(), transport.NewIO(points...))[0].(map[string]core.Primitive)
	point := core.To[map[string]core.Primitive](peak["point"])
	tests.EqualNumber(t, core.To[float64](point["x"]), 1)
	tests.EqualNumber(t, core.To[float64](point["y"]), 1)
}
