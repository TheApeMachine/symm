package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCryptoThesis(t *testing.T) {
	Convey("Given Crypto as the terminal thesis consumer", t, func() {
		crypto := &Crypto{}
		thesis := types.NewThesis(nil)

		Convey("It should consume the thesis without republishing it", func() {
			So(crypto.thesis(thesis), ShouldBeNil)
		})
	})
}
