package perspectives

import (
	"math/rand"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalizeSearchDocument(t *testing.T) {
	Convey("Given an empty search document", t, func() {
		document := normalizeSearchDocument(
			Document{},
			testSearchProfile(),
			rand.New(rand.NewSource(14)),
		)
		_, err := BuildStrategies(document)

		Convey("It should create a literal buildable YAML document", func() {
			So(err, ShouldBeNil)
			So(document.Version, ShouldEqual, 1)
			So(document.Playbooks, ShouldNotBeEmpty)
			So(document.Playbooks[0].Deny, ShouldNotBeNil)
		})
	})
}
