package response

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/types"
)

func TestNewBalances(testingTB *testing.T) {
	Convey("Given a configured paper quote wallet", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200.0)

		ctx := context.Background()

		Convey("When NewBalances constructs the handler", func() {
			balances := NewBalances(ctx, nil, nil)

			Convey("Then it should seed the model from config", func() {
				So(balances.ctx, ShouldNotBeNil)
				So(balances.cancel, ShouldNotBeNil)
				So(balances.observers, ShouldNotBeNil)
				So(balances.quoteCurrency, ShouldEqual, "USD")
				So(balanceModelValue(balances, "asset", 0, "asset"), ShouldEqual, "USD")
				So(balanceModelValue(balances, "asset", 0, "balance"), ShouldEqual, 200.0)
			})
		})
	})
}

func TestSend(testingTB *testing.T) {
	Convey("Given a balances handler with an observer", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200.0)

		balances := NewBalances(context.Background(), nil, nil)
		recorder := newSocketRecorder()
		balances.Observe(recorder)

		Convey("When a subscribe message is handled", func() {
			message := balances.Send([]byte(`{"method":"subscribe"}`))

			Convey("Then it should mark balances active and publish a balances artifact", func() {
				So(message, ShouldNotBeNil)
				So(balances.isActive.Load(), ShouldBeTrue)

				artifact := recorder.wait(testingTB)
				role, roleErr := artifact.Role()
				scope, scopeErr := artifact.Scope()

				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "balances")
				So(scope, ShouldEqual, "USD")
			})
		})

		Convey("When an unsubscribe message is handled", func() {
			balances.isActive.Store(true)
			message := balances.Send([]byte(`{"method":"unsubscribe"}`))

			Convey("Then it should mark balances inactive", func() {
				So(message, ShouldNotBeNil)
				So(balances.isActive.Load(), ShouldBeFalse)
				So(recorder.wait(testingTB), ShouldNotBeNil)
			})
		})

		Convey("When an add_order cannot be funded", func() {
			message := balances.Send([]byte(
				`{"method":"add_order","data":{"params":{"limit_price":201}}}`,
			))

			Convey("Then it should publish an insufficient-funds artifact", func() {
				So(message, ShouldNotBeNil)

				artifact := recorder.wait(testingTB)
				var payload map[string]any

				So(sonic.Unmarshal(artifact.DecryptPayload(), &payload), ShouldBeNil)
				So(payload["error"], ShouldEqual, "EOrder:Insufficient funds")
				So(payload["success"], ShouldEqual, false)
			})
		})

		Convey("When an unsupported method is handled", func() {
			message := balances.Send([]byte(`{"method":"noop"}`))

			Convey("Then it should ignore the message", func() {
				So(message, ShouldBeNil)
				So(recorder.empty(), ShouldBeTrue)
			})
		})
	})
}

func TestObserve(testingTB *testing.T) {
	Convey("Given a balances handler", testingTB, func() {
		viper.Set("market.quote_currency", "USD")

		balances := NewBalances(context.Background(), nil, nil)
		recorder := newSocketRecorder()

		Convey("When Observe registers a socket", func() {
			balances.Observe(recorder)
			balances.Send([]byte(`{"method":"subscribe"}`))

			Convey("Then the socket should receive later balance artifacts", func() {
				So(recorder.wait(testingTB), ShouldNotBeNil)
			})
		})
	})
}

type socketRecorder struct {
	messages chan []byte
}

func newSocketRecorder() *socketRecorder {
	return &socketRecorder{messages: make(chan []byte, 8)}
}

func (recorder *socketRecorder) Send(message []byte) *types.SocketMessage {
	recorder.messages <- append([]byte(nil), message...)

	return &types.SocketMessage{}
}

func (recorder *socketRecorder) Observe(sockets ...types.Socket) {}

func (recorder *socketRecorder) wait(testingTB testing.TB) *datura.Artifact {
	testingTB.Helper()

	select {
	case message := <-recorder.messages:
		artifact := datura.Acquire("test", datura.APPJSON)
		_, err := artifact.Unpack(message)

		So(err, ShouldBeNil)

		return artifact
	case <-time.After(time.Second):
		testingTB.Fatal("timed out waiting for socket artifact")
		return nil
	}
}

func (recorder *socketRecorder) empty() bool {
	select {
	case <-recorder.messages:
		return false
	default:
		return true
	}
}

func balanceModelValue(balances *Balances, path ...any) any {
	var model map[string]any

	if err := sonic.Unmarshal(balances.model.DecryptPayload(), &model); err != nil {
		return nil
	}

	current := any(model)

	for _, segment := range path {
		switch typed := current.(type) {
		case map[string]any:
			key, _ := segment.(string)
			current = typed[key]
		case []any:
			index, _ := segment.(int)
			if index < 0 || index >= len(typed) {
				return nil
			}
			current = typed[index]
		default:
			return nil
		}
	}

	return current
}
