package compiler_test

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
)

/*
The training graph is the learning system itself, so it is worth knowing that
it still compiles rather than finding out when a run is started.
*/
func TestTrainingGraph(t *testing.T) {
	Convey("Given the training graph as it is authored", t, func() {
		compiler.SetDefaultDefinitionRepository(compiler.DefaultRepository())

		raw, err := os.ReadFile("../../manifest/training.json")
		So(err, ShouldBeNil)

		result, err := compiler.NewWorkbenchRunner().Compile(raw)
		So(err, ShouldBeNil)

		response, ok := result.(compiler.CompileResponse)
		So(ok, ShouldBeTrue)

		Convey("Then it compiles with nothing to report", func() {
			So(response.Diagnostics, ShouldBeEmpty)
			So(response.OK, ShouldBeTrue)
		})

		Convey("Then the map is wired from the tape through to a decision", func() {
			run, err := compiler.NewWorkbenchRunner().Compile(raw)
			So(err, ShouldBeNil)
			So(run.(compiler.CompileResponse).NodeCount, ShouldBeGreaterThan, 25)

			// Every step TRAINING.md names, in the order it names them.
			for _, node := range []string{
				"capture",   // the raw tape is recorded
				"grid",      // metrics receive what they declared
				"window",    // relationships are read over arrivals, not one update
				"affinity",  // what moves together
				"cut",       // the threshold comes from the affinities themselves
				"edges",     // the relationships that reached it
				"regions",   // the communities they form
				"standing",  // which pull hardest
				"sensory",   // the region token
				"attractor", // what the trie predicts from it
				"decision",  // wait, enter or exit
				"reinforce", // and what it learns
				"replay",    // fragments come back out of the same table
				"entry_leg", // graded on the run into ignition
				"exit_leg",  // and the run out of it
			} {
				So(string(raw), ShouldContainSubstring, "\""+node+"\"")
			}
		})
	})
}
