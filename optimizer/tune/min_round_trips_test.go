package tune

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestEffectiveMinRoundTrips(t *testing.T) {
	Convey("Given a symbol-count floor above the cap", t, func() {
		rows := make([]types.Measurement, 0, 100)

		for index := 0; index < 100; index++ {
			rows = append(rows, types.Measurement{Symbol: fmt.Sprintf("SYM%d/EUR", index)})
		}

		Convey("effectiveMinRoundTrips should cap at twelve", func() {
			So(effectiveMinRoundTrips(0, rows, "EUR"), ShouldEqual, maxEffectiveMinRoundTrips)
		})
	})
}
