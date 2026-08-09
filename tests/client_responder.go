package tests

import (
	"encoding/json"
	"fmt"
	"time"

	sdkkraken "github.com/theapemachine/api-go/v2/pkg/kraken"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests/fixtures/instrument"
	"github.com/theapemachine/symm/tests/fixtures/orderack"
)

/*
fixtureResponder owns venue responses to websocket requests written by the SDK.
*/
type fixtureResponder struct {
	conn *Conn
}

/*
Handle routes one SDK request to its simulated venue response.
*/
func (responder *fixtureResponder) Handle(message *sdkkraken.WebSocketMessage) {
	if message == nil || len(message.Bytes()) == 0 {
		return
	}

	wire := map[string]any{}

	if err := json.Unmarshal(message.Bytes(), &wire); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"tests: decode fixture websocket request",
			err,
		))
		return
	}

	method, _ := wire["method"].(string)

	switch method {
	case "subscribe":
		responder.subscribe(wire)
	case "add_order":
		responder.addOrder()
	case "":
		return
	default:
		errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("tests: unsupported fixture websocket method %q", method),
			nil,
		))
	}
}

func (responder *fixtureResponder) subscribe(wire map[string]any) {
	params, _ := wire["params"].(map[string]any)
	channel, _ := params["channel"].(string)
	now := responder.conn.currentTime().Format(time.RFC3339Nano)
	ack, err := json.Marshal(map[string]any{
		"method": "subscribe", "req_id": wire["req_id"], "success": true,
		"result":  map[string]any{"channel": channel},
		"time_in": now, "time_out": now,
	})

	if err != nil {
		panic(fmt.Errorf("tests: encode subscription acknowledgement: %w", err))
	}

	responder.conn.Publish("subscribe", ack)

	if channel != "instrument" {
		return
	}

	symbols := responder.conn.transport.getSymbols()

	if len(symbols) == 0 {
		return
	}

	for frame := range instrument.NewMarket(symbols).Generate() {
		responder.conn.Publish("instrument", frame)
	}
}

func (responder *fixtureResponder) addOrder() {
	sequence, orderID := responder.conn.transport.nextOrderIdentity()
	responder.conn.Publish("add_order", orderack.Frame(orderack.Options{
		ReqID: int64(sequence), OrderID: orderID,
		Success: true,
	}))
}
