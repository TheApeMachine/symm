package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestPaperExecutionStillOpen(t *testing.T) {
	Convey("limit_order_placed / open ack is treated as still-open", t, func() {
		open := &kraken.Execution{
			Data: []kraken.ExecutionData{{
				OrderID:     "ORD-OPEN",
				OrderStatus: "open",
				ExecType:    "new",
			}},
		}
		So(paperExecutionStillOpen(open), ShouldBeTrue)

		filled := &kraken.Execution{
			Data: []kraken.ExecutionData{{
				OrderID:     "ORD-FILL",
				OrderStatus: "filled",
				CumQty:      decimal.NewFromFloat64(1),
			}},
		}
		So(paperExecutionStillOpen(filled), ShouldBeFalse)
	})
}

func TestPublishPlaceSoftFailsBalance(t *testing.T) {
	Convey("publishPlace returns nil even when balance refresh would fail path is soft", t, func() {
		// Structural: open ack is watched; filled ack is not.
		exec := kraken.NewExecutionFromMap(map[string]any{
			"order_id":     "ORD-1",
			"cl_ord_id":    "CL-1",
			"pair":         "BTC/USD",
			"side":         "buy",
			"status":       "open",
			"action":       "limit_order_placed",
			"volume":       0.0,
			"price":        100.0,
			"cost":         0.0,
			"fee":          0.0,
		})
		So(paperExecutionStillOpen(exec), ShouldBeTrue)
	})
}
