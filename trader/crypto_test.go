package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCryptoDecisionsReady(t *testing.T) {
	Convey("Given decision payload readiness checks", t, func() {
		crypto := &Crypto{}

		Convey("It should reject nil and empty payloads", func() {
			So(crypto.decisionsReady(nil), ShouldBeFalse)
			So(crypto.decisionsReady([]types.Decision{}), ShouldBeFalse)
			So(crypto.decisionsReady([]any{}), ShouldBeFalse)
		})

		Convey("It should accept non-empty decision payloads", func() {
			So(crypto.decisionsReady([]types.Decision{{}}), ShouldBeTrue)
		})
	})
}