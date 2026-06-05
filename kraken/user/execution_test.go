package user

import (
	"encoding/json"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/public"
)

func TestDecodeExecutions(t *testing.T) {
	Convey("Given an empty executions snapshot", t, func() {
		message := map[string]any{
			"channel": public.ExecutionsChannel,
			"type":    "snapshot",
			"data":    json.RawMessage(`[]`),
		}

		Convey("It should decode zero rows", func() {
			var rows []Execution
			So(sonic.Unmarshal(message["data"].(json.RawMessage), &rows), ShouldBeNil)
			So(len(rows), ShouldEqual, 0)
		})
	})
}
