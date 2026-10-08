package adaptive_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPathRetention(t *testing.T) {
	Convey("Given a path retention with observation-driven support retention", t, func() {
		window := adaptive.NewWindow()
		path := adaptive.NewPathRetention(window)
		obs1 := []temporal.Price{{At: 1, Value: 100}}
		obs2 := []temporal.Price{{At: 1, Value: 100}, {At: 2, Value: 101}}
		obs3 := []temporal.Price{{At: 1, Value: 100}, {At: 2, Value: 101}, {At: 3, Value: 102}}

		out := tests.CollectSeq[[]temporal.Price](path.Next(transport.NewValues(obs1, obs2, obs3).Next(nil)))
		So(path.Error(), ShouldBeNil)
		So(len(out), ShouldEqual, 3)
		So(len(out[2]), ShouldEqual, 3)
	})
}
