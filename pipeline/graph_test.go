package pipeline

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	nmruntime "github.com/theapemachine/nomagique/runtime"
)

func TestNewGraph(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewGraph is called", func() {
			graph := NewGraph()

			Convey("byGroup starts empty", func() {
				So(len(graph.byGroup), ShouldEqual, 0)
			})
		})
	})
}

func TestWrap(t *testing.T) {
	Convey("Setup", t, func() {
		graph := NewGraph()
		a := &stubNode{System: nmruntime.NewSystem(t.Context(), "cvd", nil)}

		Convey("When Wrap instruments one signal node", func() {
			wrapped := graph.Wrap("signal", []nmruntime.Node{a})

			Convey("one probed Node is returned", func() {
				So(len(wrapped), ShouldEqual, 1)
				So(wrapped[0].Name(), ShouldEqual, "cvd")
			})
		})
	})
}

func TestIngress(t *testing.T) {
	Convey("Setup", t, func() {
		graph := NewGraph()

		Convey("When Ingress records 2µs", func() {
			graph.Ingress(2 * time.Microsecond)

			Convey("ingress stats match the duration", func() {
				So(graph.ingress.n.Load(), ShouldEqual, 1)
				So(graph.ingress.lastNs.Load(), ShouldEqual, int64(2000))
				So(graph.ingress.emaNs.Load(), ShouldEqual, int64(2000))
			})
		})
	})
}

func TestTopology(t *testing.T) {
	Convey("Setup", t, func() {
		graph := NewGraph()
		a := &stubNode{System: nmruntime.NewSystem(t.Context(), "cvd", nil)}
		b := &stubNode{System: nmruntime.NewSystem(t.Context(), "model", nil)}

		graph.Wrap("signal", []nmruntime.Node{a})
		graph.Wrap("model", []nmruntime.Node{b})
		graph.Ingress(2 * time.Microsecond)

		Convey("When Topology is built", func() {
			frame := graph.Topology()
			nodes, ok := frame["nodes"].([]map[string]any)
			edges, edgesOK := frame["edges"].([]map[string]string)

			Convey("ingress skeleton omits live n", func() {
				So(ok, ShouldBeTrue)
				So(edgesOK, ShouldBeTrue)
				So(len(edges), ShouldEqual, 4)

				var ingress map[string]any
				for _, node := range nodes {
					if node["id"] == "ingress" {
						ingress = node
					}
				}
				So(ingress, ShouldNotBeNil)
				_, hasN := ingress["n"]
				So(hasN, ShouldBeFalse)
			})
		})
	})
}
