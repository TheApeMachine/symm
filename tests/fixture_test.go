package tests

import (
	"iter"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestArtifactSequence(testingTB *testing.T) {
	Convey("Given a Kraken-shaped payload sequence", testingTB, func() {
		sequence := func(yield func([]byte) bool) {
			yield([]byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"ALGO/USD"}]}`))
		}

		Convey("When artifacts are requested", func() {
			artifacts := ArtifactSequence(iter.Seq[[]byte](sequence))
			count := 0

			for artifact := range artifacts {
				count++

				role, roleErr := artifact.Role()
				scope, scopeErr := artifact.Scope()

				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "ticker")
				So(scope, ShouldEqual, "snapshot")

				artifact.Release()
			}

			Convey("Then every artifact should use channel and type directly", func() {
				So(count, ShouldEqual, 1)
			})
		})
	})
}
