package associative_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestAssociativeGrid(t *testing.T) {
	Convey("Given an Associative Grid Sympathy closure", t, func() {
		keyA := store.NewKey[any](types.Const("ticker"), types.Const("data"), types.Const("last"))

		cellA := func(in any) float64 {
			if val := keyA(in); val != nil {
				return *val
			}
			return 0
		}
		cellB := func(in any) float64 {
			if val := keyA(in); val != nil {
				return *val * 1.5 // Highly responsive cell
			}
			return 0
		}

		storeGrid := store.NewGrid[any, float64]()
		var zero any
		storeGrid(transport.NewMessage(transport.REGISTER, zero, types.Value[any, float64](cellA)))
		storeGrid(transport.NewMessage(transport.REGISTER, zero, types.Value[any, float64](cellB)))
		assocGrid := associative.NewGrid()

		So(assocGrid, ShouldNotBeNil)

		Convey("A strong impulse resolves to a spatial region", func() {
			// First tick establishes history baseline
			payload1 := map[string]any{"ticker": map[string]any{"data": []any{map[string]any{"last": 10.0}}}}
			storeGrid(transport.NewMessage[any, float64](transport.POKE, any(payload1), nil))
			impulse1 := storeGrid(transport.NewMessage[any, float64](transport.PEEK, zero, nil))
			res1 := assocGrid(impulse1)
			So(res1, ShouldNotBeNil)
			So(len(res1), ShouldEqual, 32)

			// Second tick gives movement, enabling SNR and Region calculation
			payload2 := map[string]any{"ticker": map[string]any{"data": []any{map[string]any{"last": 20.0}}}}
			storeGrid(transport.NewMessage[any, float64](transport.POKE, any(payload2), nil))
			impulse2 := storeGrid(transport.NewMessage[any, float64](transport.PEEK, zero, nil))
			res2 := assocGrid(impulse2)
			So(res2, ShouldNotBeNil)
			So(string(res2), ShouldEqual, "r1")
		})
	})
}
