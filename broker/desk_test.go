package broker

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	dashboard "github.com/theapemachine/symm/ui"
)

type recordingAccount struct {
	requests []*websocket.OrderRequest
}

func (account *recordingAccount) Submit(request *websocket.OrderRequest) error {
	account.requests = append(account.requests, request)
	return nil
}

type recordingPublisher struct {
	messages []dashboard.Message
}

func (publisher *recordingPublisher) Publish(message dashboard.Message) error {
	publisher.messages = append(publisher.messages, message)
	return nil
}

func TestDeskUpdate(t *testing.T) {
	Convey("Given a desk with two allowed buys and one USD balance snapshot", t, func() {
		viper.Set("market.quote_currency", "USD")
		account := &recordingAccount{}
		publisher := &recordingPublisher{}
		desk, err := NewDesk(context.Background(), account, publisher)

		So(err, ShouldBeNil)
		So(desk.Observe(map[string]any{
			"channel": "balances",
			"data": []map[string]any{{
				"asset":   "USD",
				"balance": 200.0,
			}},
		}), ShouldBeNil)
		So(desk.Observe(map[string]any{
			"channel": "ticker",
			"data": []map[string]any{
				{"symbol": "BTC/USD", "bid": 99.0, "ask": 100.0, "last": 100.0},
				{"symbol": "ETH/USD", "bid": 49.0, "ask": 50.0, "last": 50.0},
			},
		}), ShouldBeNil)

		first := &types.Action{
			Type:        types.ActionMarket,
			Side:        types.SideBuy,
			Symbol:      "BTC/USD",
			Fraction:    0.7,
			Allowed:     true,
			Verdict:     "allow",
			RiskStamped: true,
		}
		second := &types.Action{
			Type:        types.ActionMarket,
			Side:        types.SideBuy,
			Symbol:      "ETH/USD",
			Fraction:    0.7,
			Allowed:     true,
			Verdict:     "allow",
			RiskStamped: true,
		}

		Convey("When Desk.Update reserves quote capital for the batch", func() {
			updateErr := desk.Update([]*types.Action{first, second})

			Convey("Then only the first buy is submitted", func() {
				So(updateErr, ShouldBeNil)
				So(account.requests, ShouldHaveLength, 1)
				So(account.requests[0].String("symbol"), ShouldEqual, "BTC/USD")
				So(publisher.messages, ShouldHaveLength, 1)
				So(publisher.messages[0].Diagnostic, ShouldNotBeNil)
				So(publisher.messages[0].Diagnostic.Symbol, ShouldEqual, "ETH/USD")
			})
		})
	})
}
