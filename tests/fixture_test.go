package tests

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
)

func TestNewFixture(testingTB *testing.T) {
	Convey("Given embedded Kraken fixtures", testingTB, func() {
		ticker := NewFixture(FixtureTypeTicker)

		So(len(ticker.Data), ShouldBeGreaterThan, 0)

		Convey("It should map role and instrument scope from the payload", func() {
			artifact := ticker.ToArtifact()

			So(artifact, ShouldNotBeNil)

			defer artifact.Release()

			role, roleErr := artifact.Role()
			scope, scopeErr := artifact.Scope()

			So(roleErr, ShouldBeNil)
			So(scopeErr, ShouldBeNil)
			So(role, ShouldEqual, "ticker")
			So(scope, ShouldEqual, "update")
		})

		Convey("It should insert into the tree with the websocket prefix", func() {
			tree := dmt.NewTree("")
			ticker.Ingest(tree, 1)

			count := 0

			for range tree.Seek([]byte("ticker/update/")) {
				count++
			}

			So(count, ShouldEqual, 1)
		})
	})
}
