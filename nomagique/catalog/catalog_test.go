package catalog_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/catalog"
	"github.com/theapemachine/symm/nomagique/catalog/scan"
)

func TestPrimitives(t *testing.T) {
	Convey("Given the generated catalog", t, func() {
		primitives, err := catalog.Primitives()

		So(err, ShouldBeNil)

		Convey("Then it describes the library it was generated from", func() {
			So(primitives, ShouldNotBeEmpty)
		})

		/*
			The catalog is a committed artifact, which is the only way a deployed
			binary can answer without a source tree — and the only way such an
			artifact stays true is if something notices when it stops being. This
			regenerates it against the tree beside it: a constructor added,
			removed, renamed, or re-signatured without running
			`go run ./tools/nomagiquecatalog` fails here rather than reaching the
			editor as a palette describing a library that no longer exists.
		*/
		Convey("When the source is scanned again", func() {
			fresh, err := scan.Tree("..")

			So(err, ShouldBeNil)

			Convey("Then the committed catalog matches it", func() {
				So(len(fresh), ShouldEqual, len(primitives))

				for op, schema := range fresh {
					committed, ok := primitives[op]

					So(ok, ShouldBeTrue)
					So(committed, ShouldResemble, schema)
				}
			})
		})
	})
}
