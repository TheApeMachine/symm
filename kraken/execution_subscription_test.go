package kraken

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExecutionSubscriptionMarshal(t *testing.T) {
	Convey("ExecutionSubscription encodes private executions subscribe", t, func() {
		raw, err := sonic.Marshal(NewExecutionSubscription("tok-1"))
		So(err, ShouldBeNil)
		var decoded map[string]any
		So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)
		So(decoded["method"], ShouldEqual, "subscribe")
		params := decoded["params"].(map[string]any)
		So(params["channel"], ShouldEqual, "executions")
		So(params["token"], ShouldEqual, "tok-1")
	})
}
