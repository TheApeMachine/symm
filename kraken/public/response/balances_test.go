package response

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
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
				So(balanceModelValue(balances, "channel"), ShouldEqual, "balances")
				So(balanceModelValue(balances, "type"), ShouldEqual, "snapshot")
				So(balanceModelValue(balances, "data", 0, "asset"), ShouldEqual, "USD")
				So(balanceModelValue(balances, "data", 0, "balance"), ShouldEqual, 200.0)
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
			message := balances.Send(testArtifact(`{"method":"subscribe"}`))

			Convey("Then it should mark balances active and publish a balances artifact", func() {
				So(message, ShouldNotBeNil)
				So(balances.isActive.Load(), ShouldBeTrue)

				artifact := recorder.wait(testingTB)
				role, roleErr := artifact.Role()
				scope, scopeErr := artifact.Scope()

				So(roleErr, ShouldBeNil)
				So(scopeErr, ShouldBeNil)
				So(role, ShouldEqual, "balances")
				So(scope, ShouldEqual, "snapshot")
				So(balancePayloadValue(artifact, "channel"), ShouldEqual, "balances")
				So(balancePayloadValue(artifact, "type"), ShouldEqual, "snapshot")
				So(balancePayloadValue(artifact, "data", 0, "balance"), ShouldEqual, 200.0)
				So(balancePayloadValue(message, "channel"), ShouldEqual, "balances")
				So(balancePayloadValue(message, "type"), ShouldEqual, "snapshot")
				So(balancePayloadValue(message, "data"), ShouldNotBeEmpty)
			})
		})

		Convey("When an unsubscribe message is handled", func() {
			balances.isActive.Store(true)
			message := balances.Send(testArtifact(`{"method":"unsubscribe"}`))

			Convey("Then it should mark balances inactive", func() {
				So(message, ShouldNotBeNil)
				So(balances.isActive.Load(), ShouldBeFalse)
				So(recorder.empty(), ShouldBeTrue)
				So(balancePayloadValue(message, "method"), ShouldEqual, "unsubscribe")
				So(balancePayloadValue(message, "success"), ShouldEqual, true)
			})
		})

		Convey("When an add_order is handled", func() {
			message := balances.Send(testArtifact(
				`{"method":"add_order","params":{"limit_price":201}}`,
			))

			Convey("Then it should not mutate balances before a filled execution", func() {
				So(message, ShouldBeNil)
				So(balanceModelValue(balances, "data", 0, "balance"), ShouldEqual, 200.0)
				So(recorder.empty(), ShouldBeTrue)
			})
		})

		Convey("When a filled buy execution is handled", func() {
			message := balances.Send(testArtifact(
				`{"channel":"executions","type":"update","data":[{"symbol":"AI/USD","side":"buy","order_status":"filled","order_qty":2,"last_price":10,"fee":0.08,"fee_ccy":"USD"}]}`,
			))

			Convey("Then it should apply notional and fees to the paper ledger", func() {
				So(message, ShouldNotBeNil)

				So(balancePayloadValue(message, "type"), ShouldEqual, "update")
				So(balancePayloadValue(message, "data", 0, "asset"), ShouldEqual, "USD")
				So(balancePayloadValue(message, "data", 0, "balance"), ShouldEqual, 179.92)
				So(balancePayloadValue(message, "data", 1, "asset"), ShouldEqual, "AI")
				So(balancePayloadValue(message, "data", 1, "balance"), ShouldEqual, 2.0)
			})
		})

		Convey("When an unsupported method is handled", func() {
			message := balances.Send(testArtifact(`{"method":"noop"}`))

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
			balances.Send(testArtifact(`{"method":"subscribe"}`))

			Convey("Then the socket should receive later balance artifacts", func() {
				So(recorder.wait(testingTB), ShouldNotBeNil)
			})
		})
	})
}

func TestBalancesPublishInternalLedgerToUI(testingTB *testing.T) {
	Convey("Given a balances handler on the shared pool", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200.0)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 8, nil)
		balances := NewBalances(ctx, pool, nil)
		ui := pool.Subscribe("ui", nil)

		Convey("When a filled buy execution updates the paper ledger", func() {
			message := balances.Send(testArtifact(
				`{"channel":"executions","type":"update","data":[{"symbol":"SYN/USD","side":"buy","order_status":"filled","order_qty":1,"last_price":0.54,"fee":0.00216,"fee_ccy":"USD"}]}`,
			))

			Convey("Then the same balance update should reach the UI group", func() {
				So(message, ShouldNotBeNil)

				waitCtx, cancel := context.WithTimeout(ctx, time.Second)
				defer cancel()

				artifact, err := ui.Wait(waitCtx)
				So(err, ShouldBeNil)
				So(artifact, ShouldNotBeNil)

				role, roleErr := artifact.Role()
				destination, destinationErr := artifact.Destination()
				So(roleErr, ShouldBeNil)
				So(destinationErr, ShouldBeNil)
				So(role, ShouldEqual, "balances")
				So(destination, ShouldEqual, "ui")
				So(balancePayloadValue(artifact, "type"), ShouldEqual, "update")
				So(balancePayloadValue(artifact, "data", 0, "balance"), ShouldEqual, 199.45784)
				So(balancePayloadValue(artifact, "data", 1, "asset"), ShouldEqual, "SYN")
			})
		})
	})
}

type socketRecorder struct {
	messages chan *datura.Artifact
}

func newSocketRecorder() *socketRecorder {
	return &socketRecorder{messages: make(chan *datura.Artifact, 8)}
}

func (recorder *socketRecorder) Send(artifact *datura.Artifact) *datura.Artifact {
	recorder.messages <- artifact

	return artifact
}

func (recorder *socketRecorder) Observe(sockets ...types.Socket) {}

func (recorder *socketRecorder) wait(testingTB testing.TB) *datura.Artifact {
	testingTB.Helper()

	select {
	case artifact := <-recorder.messages:
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

func testArtifact(payload string) *datura.Artifact {
	return datura.Acquire("test", datura.APPJSON).WithPayload([]byte(payload))
}

func balancePayloadValue(artifact *datura.Artifact, path ...any) any {
	var payload map[string]any

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &payload); err != nil {
		return nil
	}

	current := any(payload)

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
