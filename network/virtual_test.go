package network

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
)

func withVirtual(f func(*Virtual)) func() {
	return func() {
		prevQuote := system.Cfg.Market.QuoteCurrency
		prevBal := system.Cfg.Market.Balance
		system.Cfg.Market.QuoteCurrency = "USD"
		system.Cfg.Market.Balance = 1000

		defer func() {
			system.Cfg.Market.QuoteCurrency = prevQuote
			system.Cfg.Market.Balance = prevBal
		}()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f(NewVirtual(ctx))
	}
}

func TestWrite(t *testing.T) {
	Convey("Setup", t, withVirtual(func(virtual *Virtual) {
		cases := []struct {
			name    string
			message map[string]any
			wantErr bool
		}{
			{
				name:    "garbage is rejected",
				message: nil,
				wantErr: true,
			},
			{
				name:    "unknown method is rejected",
				message: map[string]any{"method": "cancel_order"},
				wantErr: true,
			},
			{
				name: "subscribe balances is accepted",
				message: map[string]any{
					"method": "subscribe",
					"params": map[string]any{"channel": "balances"},
				},
				wantErr: false,
			},
			{
				name: "subscribe executions is accepted",
				message: map[string]any{
					"method": "subscribe",
					"params": map[string]any{"channel": "executions"},
				},
				wantErr: false,
			},
			{
				name: "add_order is accepted",
				message: map[string]any{
					"method": "add_order",
					"params": map[string]any{
						"side": "buy", "symbol": "AAA/USD", "order_qty": "1",
					},
				},
				wantErr: false,
			},
		}

		for _, tc := range cases {
			Convey("When "+tc.name, func() {
				var (
					buf []byte
					err error
				)

				if tc.message == nil {
					err = virtual.Write([]byte("{"))
				} else {
					buf, err = sonic.Marshal(tc.message)
					So(err, ShouldBeNil)
					err = virtual.Write(buf)
				}

				if tc.wantErr {
					So(err, ShouldNotBeNil)
					return
				}

				So(err, ShouldBeNil)

				frame, readErr := virtual.Read()
				So(readErr, ShouldBeNil)
				So(len(frame), ShouldBeGreaterThan, 0)
			})
		}

		Convey("When subscribe balances is read back as a wallet", func() {
			buf, err := sonic.Marshal(map[string]any{
				"method": "subscribe",
				"params": map[string]any{"channel": "balances"},
			})
			So(err, ShouldBeNil)
			So(virtual.Write(buf), ShouldBeNil)

			frame, err := virtual.Read()
			So(err, ShouldBeNil)

			var balance market.Balance
			So(sonic.Unmarshal(frame, &balance), ShouldBeNil)
			So(balance.Quote, ShouldEqual, "USD")
			So(balance.Assets, ShouldNotBeEmpty)
			So(balance.Assets[0].Capital.String(), ShouldEqual, "1000")
		})
	}))
}

func TestPost(t *testing.T) {
	Convey("Setup", t, withVirtual(func(virtual *Virtual) {
		cases := []struct {
			name    string
			url     string
			wantErr bool
		}{
			{"Balance path", "/0/private/Balance", false},
			{"TradeBalance path", "/0/private/TradeBalance", false},
			{"unknown path", "/0/private/OpenOrders", true},
		}

		for _, tc := range cases {
			Convey("When posting "+tc.name, func() {
				raw, err := virtual.Post(tc.url, nil)

				if tc.wantErr {
					So(err, ShouldNotBeNil)
					So(raw, ShouldBeNil)
					return
				}

				So(err, ShouldBeNil)

				var balance market.Balance
				So(json.Unmarshal(raw, &balance), ShouldBeNil)
				So(balance.Quote, ShouldEqual, "USD")
				So(balance.Cash("USD").String(), ShouldEqual, "1000")
			})
		}
	}))
}

func TestRead(t *testing.T) {
	Convey("Setup", t, withVirtual(func(virtual *Virtual) {
		Convey("When the context is cancelled before a frame arrives", func() {
			ctx, cancel := context.WithCancel(context.Background())
			virtual = NewVirtual(ctx)
			cancel()

			_, err := virtual.Read()
			So(err, ShouldNotBeNil)
		})
	}))
}
