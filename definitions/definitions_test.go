package definitions_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/definitions"
)

func TestLoad(t *testing.T) {
	Convey("Given definition names", t, func() {
		Convey("Loading system graph", func() {
			graph, err := definitions.Load("system")
			So(err, ShouldBeNil)
			So(graph.ID, ShouldEqual, "system:orchestration")
			So(len(graph.Nodes), ShouldBeGreaterThan, 0)
		})

		Convey("Loading logic graph", func() {
			graph, err := definitions.Load("logic")
			So(err, ShouldBeNil)
			So(graph.ID, ShouldEqual, "logic:stage")
			So(len(graph.Nodes), ShouldBeGreaterThan, 0)
		})

		Convey("Loading unknown graph returns error", func() {
			_, err := definitions.Load("non_existent_graph_12345")
			So(err, ShouldNotBeNil)
		})
	})
}
