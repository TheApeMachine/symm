package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestRlsPriorNext(t *testing.T) {
	Convey("Given an affine design and a configured coefficient variance", t, func() {
		node := newRLSPrior(store.NewConstant(core.From(9.0)))
		output, err := transport.Evaluate[map[string]core.Primitive](node,
			core.Record(map[string]any{"design": []float64{1, 2, -3}}))
		So(err, ShouldBeNil)
		So(core.To[[]float64](output["beta"]), ShouldResemble, []float64{0, 0, 0})
		So(core.To[[][]float64](output["root"]), ShouldResemble, [][]float64{{3, 0, 0}, {0, 3, 0}, {0, 0, 3}})

		Convey("An invalid prior variance cannot create a model", func() {
			invalid := newRLSPrior(store.NewConstant(core.From(-1.0)))
			_, err := transport.Evaluate[map[string]core.Primitive](invalid,
				core.Record(map[string]any{"design": []float64{1, 2, -3}}))
			So(err, ShouldNotBeNil)
		})
	})
}
