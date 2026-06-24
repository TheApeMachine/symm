package tests

import (
	"testing"
	"time"

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
			at := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC).UnixNano()
			ticker.Ingest(tree, at)

			artifact := ticker.ToArtifact()
			So(artifact, ShouldNotBeNil)

			defer artifact.Release()

			artifact.SetTimestamp(at)
			count := 0

			for range tree.Seek(artifact.Prefix("timestamp")) {
				count++
			}

			So(count, ShouldEqual, 1)
		})
	})
}
