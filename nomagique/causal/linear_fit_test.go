package causal_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/causal"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestLinearFitNext(t *testing.T) {
	Convey("LinearFit estimates parameters from adapter", t, func() {
		node := causal.NewLinearFit(1e-15)

		state := data.NewState(data.NewMap("target", "target", "feature", "feature"))
		adapter := data.NewAdapter(nil, state)
		input := data.NewOutputMap()
		input.Values["target"] = 5.0
		input.Values["feature"] = 1.0

		for range adapter.Next(data.NewValue(input)) {
		}

		data.Read[*data.Adapter](node.Next(data.NewValue(adapter)))
		So(node.Error(), ShouldBeNil)
	})
}
