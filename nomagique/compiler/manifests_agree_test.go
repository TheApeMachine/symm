package compiler_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
)

/*
Every authored graph has to have two halves that agree, not just the one
being worked on.
*/
func TestEveryManifestAgrees(t *testing.T) {
	Convey("Given every graph in the manifest directory", t, func() {
		paths, err := filepath.Glob("../../manifest/*.json")
		So(err, ShouldBeNil)
		So(len(paths), ShouldBeGreaterThan, 0)

		for _, path := range paths {
			raw, err := os.ReadFile(path)
			So(err, ShouldBeNil)

			_, err = compiler.ParseGraph(raw)

			Convey("Then "+filepath.Base(path)+" agrees with itself", func() {
				So(err, ShouldBeNil)
			})
		}
	})
}
