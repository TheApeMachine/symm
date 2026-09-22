package compiler_test

import (
	"os"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
)

/*
The training graph is the learning system itself.

Proving it compiles proves almost nothing: a node whose producer forgot to
declare the edge still compiles, still runs, and quietly trains on whatever
its arguments happened to hold. What has to be proved is that the path
claimed in TRAINING.md is actually wired, step by step.
*/
func TestTrainingGraph(t *testing.T) {
	Convey("Given the training graph as it is authored", t, func() {
		compiler.SetDefaultDefinitionRepository(compiler.DefaultRepository())

		raw, err := os.ReadFile("../../manifest/training.json")
		So(err, ShouldBeNil)

		graph, err := compiler.ParseGraph(raw)
		So(err, ShouldBeNil)

		program, err := compiler.Compile(graph, nil, compiler.DefaultRepository())
		So(err, ShouldBeNil)

		node := func(id string) compiler.CompiledNode {
			index, present := program.NodeMap[id]
			So(present, ShouldBeTrue)

			return program.Nodes[index]
		}

		// Whether one node's field actually reaches another's, as a route in
		// the compiled program rather than as a line in the JSON.
		carries := func(from, fromField, to, toField string) bool {
			producer := node(from)
			consumer := node(to)

			for _, route := range program.Routes {
				if route.FromNode != producer.Index || route.ToNode != consumer.Index {
					continue
				}

				if route.FromField != producer.Outputs[fromField].Index {
					continue
				}

				if route.ToField != consumer.Inputs[toField].Index {
					continue
				}

				return true
			}

			return false
		}

		Convey("Then the tape reaches the record and the metrics", func() {
			So(carries("feed", "read", "capture", "payload"), ShouldBeTrue)
			So(carries("feed", "read", "grid", "data"), ShouldBeTrue)
			So(carries("replay", "out", "grid", "data"), ShouldBeTrue)
		})

		Convey("Then the map is measured over arrivals rather than one update", func() {
			So(carries("grid", "values", "window", "value"), ShouldBeTrue)
			So(carries("grid", "present", "window", "present"), ShouldBeTrue)
			So(carries("window", "out", "affinity", "data"), ShouldBeTrue)
			So(carries("window", "rows", "affinity", "rows"), ShouldBeTrue)
		})

		Convey("Then what counts as a strong relationship is read off the affinities", func() {
			So(carries("affinity", "corr", "spread", "value"), ShouldBeTrue)
			So(carries("spread", "median", "cut", "a"), ShouldBeTrue)
			So(carries("spread", "medianAbsolute", "cut", "b"), ShouldBeTrue)
			So(carries("cut", "out", "edges", "threshold"), ShouldBeTrue)
			So(carries("affinity", "corr", "edges", "matrix"), ShouldBeTrue)
		})

		Convey("Then the regions and their standing come from that graph", func() {
			So(carries("edges", "fromNodes", "regions", "fromNodes"), ShouldBeTrue)
			So(carries("edges", "fromNodes", "standing", "fromNodes"), ShouldBeTrue)
			So(carries("standing", "ranks", "heat", "value"), ShouldBeTrue)
		})

		Convey("Then the token reaches the trie and a decision comes back", func() {
			So(carries("token_3", "out", "sensory", "contextBytes"), ShouldBeTrue)
			So(carries("sensory", "out", "attractor", "contextBytes"), ShouldBeTrue)
			So(carries("attractor", "class", "decision", "class"), ShouldBeTrue)
			So(carries("attractor", "prob", "decision", "prob"), ShouldBeTrue)
			So(carries("decision", "winner", "reinforce", "classBytes"), ShouldBeTrue)
		})

		/*
			The failure this exists to catch: a reader and a writer each
			holding their own learned memory. Nothing errors, nothing is
			slow, and the reader never sees a thing the writer recorded.
		*/
		Convey("Then the reader and the writer share one learned memory", func() {
			for _, id := range []string{"attractor", "reinforce"} {
				held := node(id)
				So(held.ArgsTemplate.IsValid(), ShouldBeTrue)

				pointer, err := held.ArgsTemplate.Ptr(
					uint16(held.Inputs["memory"].Offset),
				)
				So(err, ShouldBeNil)

				bound := pointer.Interface()
				So(bound.IsValid(), ShouldBeTrue)
				So(capnp.Client(bound.Client()).IsValid(), ShouldBeTrue)
			}
		})

		/*
			The failure this exists to catch: ground truth wired only on the
			consumer's side. The compiler routes from the producer, so the
			targets would receive nothing, report zero, and the system would
			train on zero-valued labels while appearing to run.
		*/
		Convey("Then ground truth reaches the targets that grade it", func() {
			So(carries("excursion", "anchor", "entry_leg", "past"), ShouldBeTrue)
			So(carries("excursion", "ignition", "entry_leg", "current"), ShouldBeTrue)
			So(carries("excursion", "ignition", "exit_leg", "past"), ShouldBeTrue)
			So(carries("excursion", "extremum", "exit_leg", "current"), ShouldBeTrue)

			for _, id := range []string{"entry_leg", "exit_leg"} {
				// A node with nothing required of it is an origin: it runs on
				// whatever its arguments held, which is zero.
				So(node(id).RequiredMask, ShouldNotEqual, 0)
			}
		})

		Convey("Then what was graded reaches the learner", func() {
			So(carries("entry_leg", "out", "graded", "target"), ShouldBeTrue)
			So(carries("surprisal", "out", "graded", "feature"), ShouldBeTrue)
		})
	})
}

/*
Roots are the nodes an evaluation begins at. Kahn's algorithm decrements
every indegree to zero on its way through, so a roots list read from the map
it leaves behind names every node in the graph — which is not wrong by a
little, it is the whole graph.
*/
func TestProgramRoots(t *testing.T) {
	Convey("Given a graph whose nodes mostly consume from others", t, func() {
		compiler.SetDefaultDefinitionRepository(compiler.DefaultRepository())

		raw, err := os.ReadFile("../../manifest/training.json")
		So(err, ShouldBeNil)

		graph, err := compiler.ParseGraph(raw)
		So(err, ShouldBeNil)

		program, err := compiler.Compile(graph, nil, compiler.DefaultRepository())
		So(err, ShouldBeNil)

		Convey("Then only the nodes nothing has to run before are roots", func() {
			So(len(program.Roots), ShouldBeGreaterThan, 0)
			So(len(program.Roots), ShouldBeLessThan, len(program.Nodes))

			rooted := make(map[string]bool, len(program.Roots))

			for _, root := range program.Roots {
				rooted[program.Nodes[root].ID] = true
			}

			// A node reading a retained store is still a root: what it reads
			// is already there, which is what lets the graph have feedback
			// without closing a cycle. A node waiting on an ordinary producer
			// is not.
			for _, waiting := range []string{
				"affinity", "spread", "cut", "edges", "regions", "standing",
				"sensory", "attractor", "decision", "reinforce", "graded",
				"entry_leg", "exit_leg",
			} {
				if waiting == "affinity" {
					// Fed by window, which is retained.
					continue
				}

				So(rooted[waiting], ShouldBeFalse)
			}

			So(rooted["feed"], ShouldBeTrue)
		})
	})
}
