package market

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

const spreadFixture = `{
	"error": [],
	"result": {
		"XXBTZUSD": [[1780081054, "73268.90000", "73270.00000"]],
		"last": 1780081055
	}
}`

func TestNewSpread(t *testing.T) {
	Convey("Given a Kraken spread payload", t, func() {
		result := SpreadResult{}

		Convey("It should unmarshal spread rows and last on the wire", func() {
			So(json.Unmarshal([]byte(spreadFixture), &public.Response{Result: &result}), ShouldBeNil)

			rows, ok := result["XXBTZUSD"].([]any)

			So(ok, ShouldBeTrue)
			So(len(rows), ShouldEqual, 1)
			So(result["last"], ShouldEqual, 1780081055)
		})
	})
}
