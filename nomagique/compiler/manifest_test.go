package compiler

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/manifest"
)

/*
Every manifest that ships must compile, so a graph referencing a primitive
that does not exist is caught here rather than at boot.
*/
func TestManifestsCompile(t *testing.T) {
	Convey("Given the manifests that ship with the system", t, func() {
		identifiers, err := manifest.List()
		So(err, ShouldBeNil)
		So(len(identifiers), ShouldBeGreaterThan, 0)

		for _, identifier := range identifiers {
			Convey("It compiles "+identifier, func() {
				graph, err := DefaultRepository().Load(identifier)
				So(err, ShouldBeNil)

				program, err := Compile(graph, nil, DefaultRepository())
				So(err, ShouldBeNil)

				// A graph describes work to do or a surface to show it on, so
				// one that lowered to neither did not describe anything.
				surfaces := 0

				if program.UI != nil {
					surfaces = len(program.UI.Routes)
				}

				So(len(program.Nodes)+surfaces, ShouldBeGreaterThan, 0)
			})
		}
	})
}
