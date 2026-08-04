package kraken_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
)

func TestNewExecutionFromMap(t *testing.T) {
	Convey("Given a paper execution carrying its client order identifier", t, func() {
		execution := kraken.NewExecutionFromMap(datura.Map[any]{
			"cl_ord_id": "exit-order-id",
			"pair":      "SLAY/USD",
			"side":      "sell",
		})

		Convey("The normalized execution should preserve order correlation", func() {
			So(execution.Data, ShouldHaveLength, 1)
			So(execution.Data[0].ClientOrderID, ShouldEqual, "exit-order-id")
		})
	})
}
