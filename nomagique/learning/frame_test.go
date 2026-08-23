package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestFramePrimitive(t *testing.T) {
	Convey("Given a dense resonance manifold behind a Frame boundary", t, func() {
		manifold := NewResonanceManifold([]int{2, 2}, 1, 0.03)
		stream := nomagique.NewStream(FramePrimitive(manifold, false), types.Frame{})
		input := types.Frame{}
		input.Put(SymbolFeatureCount, 2)
		input.Put(FeatureSymbol(0), 0.25)
		input.Put(FeatureSymbol(1), -0.5)
		output, err := stream.Step(input)

		Convey("It should publish settled diagnostics without string-keyed transport", func() {
			So(err, ShouldBeNil)
			So(output.Has(SymbolEnergy), ShouldBeTrue)
			So(output.Has(SymbolSurprise), ShouldBeTrue)
			So(output.MustGet(SymbolLatentCount), ShouldEqual, 2)
			So(stream.Project().MustGet(SymbolInvocation), ShouldEqual, 1)
		})
	})
}
