package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestGrid(t *testing.T) {
	Convey("Given a virtual grid with distributed cells", t, func() {
		keyA := store.NewKey[any](types.Const("ticker"), types.Const("data"), types.Const("last"))
		keyB := store.NewKey[any](types.Const("trade"), types.Const("price"))

		cellA := func(in any) float64 {
			// Expecting an extracted []*float64
			if extracted, ok := in.([]*float64); ok && len(extracted) > 0 && extracted[0] != nil {
				return *extracted[0] * 2.0
			}
			return 0
		}

		cellB := func(in any) float64 {
			if extracted, ok := in.([]*float64); ok && len(extracted) > 0 && extracted[0] != nil {
				return *extracted[0] + 10.0
			}
			return 0
		}

		grid := store.NewGrid[any, float64]()
		var zero any
		
		grid(transport.NewMessage(
			transport.REGISTER, 
			any([]types.Value[any, *float64]{types.Value[any, *float64](keyA)}), 
			types.Value[any, float64](cellA),
		))
		grid(transport.NewMessage(
			transport.REGISTER, 
			any([]types.Value[any, *float64]{types.Value[any, *float64](keyB)}), 
			types.Value[any, float64](cellB),
		))

		Convey("Incoming ticker events route to matching cell and transform metric", func() {
			tickerPayload := map[string]any{
				"ticker": map[string]any{
					"data": []any{
						map[string]any{"last": 50.0},
					},
				},
			}

			grid(transport.NewMessage[any, float64](transport.POKE, any(tickerPayload), nil))
			results := grid(transport.NewMessage[any, float64](transport.PEEK, zero, nil))
			So(results[0], ShouldEqual, 100.0)
			So(results[1], ShouldEqual, 0.0)
		})

		Convey("Incoming trade events route to trade cell", func() {
			tradePayload := map[string]any{
				"trade": map[string]any{
					"price": 42.0,
				},
			}

			grid(transport.NewMessage[any, float64](transport.POKE, any(tradePayload), nil))
			results := grid(transport.NewMessage[any, float64](transport.PEEK, zero, nil))
			So(results[0], ShouldEqual, 0.0)
			So(results[1], ShouldEqual, 52.0)
		})

		Convey("JSON byte payload routes and extracts correctly", func() {
			rawJSON := []byte(`{"ticker":{"data":[{"last":75.5}]}}`)
			grid(transport.NewMessage[any, float64](transport.POKE, any(rawJSON), nil))
			results := grid(transport.NewMessage[any, float64](transport.PEEK, zero, nil))
			So(results[0], ShouldEqual, 151.0)
			So(results[1], ShouldEqual, 0.0)
		})
	})
}
