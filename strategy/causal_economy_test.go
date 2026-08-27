package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/logic/causal"
	"github.com/theapemachine/symm/nomagique/relation"
)

/*
topoCoordinate builds a market coordinate identity for ordering tests. The
metric names are chosen so that plain alphabetical order (a_leaf < b_mid <
c_mid < d_root_b < e_root_a) is the exact reverse of the causal parent-first
order (e_root_a < d_root_b < c_mid < b_mid < a_leaf), forcing the sort to
respect the DAG rather than accidentally passing lexicographically.
*/
func topoCoordinate(metric string) relation.Coordinate {
	return relation.Coordinate{
		Symbol: "TEST/USD",
		Source: "test",
		Metric: metric,
		Epoch:  1,
	}
}

/*
topoTransition builds one identified transition declaring a self history and
an optional parent. It is the minimal transition the ordering algorithm reads.
*/
func topoTransition(target relation.Coordinate, parents ...relation.Coordinate) *causal.TransitionModel {
	allowed := make([]causal.AllowedParent, 0, len(parents))

	for _, parent := range parents {
		allowed = append(allowed, causal.AllowedParent{
			Parent: causal.VariableID{Coordinate: parent, Role: causal.RoleMarket},
			Lag:    time.Second,
		})
	}

	return &causal.TransitionModel{
		Target:          causal.VariableID{Coordinate: target, Role: causal.RoleMarket},
		SelfLag:         time.Second,
		Parents:         allowed,
		ResidualVariance: 0,
		Status:          causal.IdentificationIdentified,
	}
}

func TestTransitionOrder(t *testing.T) {
	Convey("Given a fitted transition set with a multi-tier cascade", t, func() {
		// Layer 1 autonomous: A, B. Layer 2: C ← A, D ← A,B. Layer 3: E ← C,D.
		coordA := topoCoordinate("e_root_a")
		coordB := topoCoordinate("d_root_b")
		coordC := topoCoordinate("c_mid")
		coordD := topoCoordinate("b_mid")
		coordE := topoCoordinate("a_leaf")

		state := &CausalState{
			Transitions: map[relation.Coordinate]*causal.TransitionModel{
				coordA: topoTransition(coordA),
				coordB: topoTransition(coordB),
				coordC: topoTransition(coordC, coordA),
				coordD: topoTransition(coordD, coordA, coordB),
				coordE: topoTransition(coordE, coordC, coordD),
			},
		}

		Convey("every parent precedes its child in the returned order", func() {
			order := state.transitionOrder()
			So(len(order), ShouldEqual, 5)

			position := map[relation.Coordinate]int{}

			for index, coordinate := range order {
				position[coordinate] = index
			}

			So(position[coordA], ShouldBeLessThan, position[coordC])
			So(position[coordA], ShouldBeLessThan, position[coordD])
			So(position[coordB], ShouldBeLessThan, position[coordD])
			So(position[coordC], ShouldBeLessThan, position[coordE])
			So(position[coordD], ShouldBeLessThan, position[coordE])
		})

		Convey("the order is deterministic across repeated calls", func() {
			first := state.transitionOrder()
			second := state.transitionOrder()

			So(len(first), ShouldEqual, len(second))

			for index := range first {
				So(first[index], ShouldEqual, second[index])
			}
		})
	})

	Convey("Given a transition set with a cycle", t, func() {
		coordA := topoCoordinate("a_cycle")
		coordB := topoCoordinate("b_cycle")

		state := &CausalState{
			Transitions: map[relation.Coordinate]*causal.TransitionModel{
				coordA: topoTransition(coordA, coordB),
				coordB: topoTransition(coordB, coordA),
			},
		}

		Convey("the order still returns every coordinate exactly once", func() {
			order := state.transitionOrder()
			So(len(order), ShouldEqual, 2)
			So(order[0] != order[1], ShouldBeTrue)
		})
	})
}
