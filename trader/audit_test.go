package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAudit(t *testing.T) {
	Convey("Given audit", t, func() {
		Convey("It should not panic for structured fields", func() {
			audit("test_event", map[string]any{
				"symbol": "BTC/EUR",
				"edge":   0.01,
			})
		})
	})
}
