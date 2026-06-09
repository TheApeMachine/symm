package market

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/types"
)

func TestNewLevel3Params(t *testing.T) {
	Convey("Given symbols for level3 subscribe", t, func() {
		params := NewLevel3Params([]string{"BTC/EUR", "ADA/EUR"})

		Convey("It should build the level3 channel payload", func() {
			So(params.Channel, ShouldEqual, "level3")
			So(params.Symbol, ShouldResemble, []string{"BTC/EUR", "ADA/EUR"})
			So(params.Snapshot, ShouldBeTrue)
		})

		Convey("It should carry a token on the params", func() {
			params.Token = "venue-token"

			So(params.Token, ShouldEqual, "venue-token")
		})
	})
}

func TestLevel3UpdateUnmarshal(t *testing.T) {
	Convey("Given a level3 socket payload array", t, func() {
		message := types.NewSocketMessage()
		message.Data = []byte(`[{"checksum":2348626433,"symbol":"BTC/EUR","bids":[],"asks":[]}]`)

		level3Updates := make([]Level3Update, 0)

		err := message.Unmarshal(&level3Updates)

		Convey("It should decode the first update", func() {
			So(err, ShouldBeNil)
			So(len(level3Updates), ShouldEqual, 1)
			So(level3Updates[0].Symbol, ShouldEqual, "BTC/EUR")
			So(level3Updates[0].Checksum, ShouldEqual, 2348626433)
		})
	})
}

func TestNewKrakenMessageLevel3Token(t *testing.T) {
	Convey("Given level3 subscribe params with a token", t, func() {
		params := NewLevel3Params([]string{"BTC/EUR"})
		params.Token = "venue-token"

		frame, err := types.NewKrakenMessage("subscribe", params, 7)

		Convey("It should marshal the token onto the wire frame", func() {
			So(err, ShouldBeNil)
			So(string(frame.Params.(json.RawMessage)), ShouldContainSubstring, `"token":"venue-token"`)
		})
	})
}
