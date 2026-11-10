package broker

import (
	"context"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
)

func TestLiveEnterRequiresArmed(t *testing.T) {
	Convey("real Enter refuses until Live.Arm succeeds", t, func() {
		live := &Live{}
		tier := &Tier{
			Type:      TierTypeReal,
			live:      live,
			unlocked:  true,
			positions: NewCache(context.Background(), "real", nil, nil, nil, nil),
		}

		err := tier.Enter("BTC/USD", "path", true)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "real transport not armed")

		live.armed.Store(true)

		// Armed but no venue — past the arm gate into sizing.
		err = tier.Enter("BTC/USD", "path", true)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldNotContainSubstring, "real transport not armed")
	})
}

func TestLiveSeal(t *testing.T) {
	Convey("Seal attaches the session token and a numeric order quantity", t, func() {
		live := &Live{
			token: func(context.Context) (string, error) {
				return "session-token", nil
			},
		}

		raw, err := sonic.Marshal(map[string]any{
			"method": "add_order",
			"params": map[string]any{
				"symbol":     "BTC/USD",
				"side":       "buy",
				"order_type": "market",
				"order_qty":  "1.25",
				"cl_ord_id":  "abc",
			},
		})
		So(err, ShouldBeNil)

		sealed, err := live.Seal(raw)
		So(err, ShouldBeNil)

		var got struct {
			Method string `json:"method"`
			Params struct {
				Token    string  `json:"token"`
				OrderQty float64 `json:"order_qty"`
				ClOrdID  string  `json:"cl_ord_id"`
			} `json:"params"`
		}

		So(sonic.Unmarshal(sealed, &got), ShouldBeNil)
		So(got.Method, ShouldEqual, "add_order")
		So(got.Params.Token, ShouldEqual, "session-token")
		So(got.Params.OrderQty, ShouldEqual, 1.25)
		So(got.Params.ClOrdID, ShouldEqual, "abc")
	})

	Convey("Seal returns the token error instead of an unsigned order", t, func() {
		live := &Live{
			token: func(context.Context) (string, error) {
				return "", errnie.Err(errnie.IO, "no token", nil)
			},
		}

		raw := []byte(`{"method":"add_order","params":{"order_qty":"1"}}`)
		sealed, err := live.Seal(raw)

		So(err, ShouldNotBeNil)
		So(sealed, ShouldBeNil)
	})
}
