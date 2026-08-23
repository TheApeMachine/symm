package nomagique

import (
	"testing"

	"github.com/theapemachine/symm/nomagique/types"
	. "github.com/smartystreets/goconvey/convey"
)

func TestKeyedStreamsCompatibility(t *testing.T) {
	Convey("KeyedStreams delegates to the canonical Number implementation", t, func() {
		collection := NewKeyedStreams[string](numberAccumulator, func(key string) types.Frame {
			return types.Frame{}.Set(numberTotal, float64(len(key)))
		})
		output, err := collection.Step("AA", types.Frame{}.Set(numberDelta, 3))
		So(err, ShouldBeNil)
		So(output.MustGet(numberTotal), ShouldEqual, 5.0)
		state, found := collection.Project("AA")
		So(found, ShouldBeTrue)
		So(state.MustGet(numberTotal), ShouldEqual, 5.0)
		collection.Delete("AA")
		_, found = collection.Project("AA")
		So(found, ShouldBeFalse)
	})
}
