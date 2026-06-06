package reasoning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

func TestKeyOf(t *testing.T) {
	Convey("Given a reasoning forest", t, func() {
		forest := Seeds(DeriveVocabulary(ignitionRows()))[0]
		key := keyOf(forest)

		Convey("It should preserve identity across a YAML round trip", func() {
			encoded, err := reasoning.MarshalThoughts(forest, 2)
			So(err, ShouldBeNil)

			decoded, err := reasoning.ParseThoughts(encoded)
			So(err, ShouldBeNil)
			So(keyOf(decoded), ShouldEqual, key)
		})

		Convey("It should distinguish threshold changes", func() {
			changed := cloneForest(forest)
			changed[0].When.All[1].Value = 2

			So(keyOf(changed), ShouldNotEqual, key)
		})

		Convey("It should distinguish action parameter changes", func() {
			changed := cloneForest(forest)
			changed[1].Do.Offset = 0.05

			So(keyOf(changed), ShouldNotEqual, key)
		})
	})
}
