package controlflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/network/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
TestDiscoveryAndSubscriptionCircuit verifies the complete discovery and subscription
circuit designed in the architecture diagram:
1. WebSocketClient connects to the endpoint.
2. When status becomes READY, Condition.ready fires true.
3. Condition.ready triggers Once, passing the discovery JSON payload through to WebSocketClient.write.
4. The mock venue responds with an instrument snapshot.
5. Match detects the "instrument" snapshot from WebSocketClient.read and activates.
6. The subscription items are batched and paced through Delay.
*/
func TestDiscoveryAndSubscriptionCircuit(t *testing.T) {
	Convey("Given the WebSocket discovery and subscription circuit", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		upgrader := gorillaws.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}

		receivedMessages := make([]string, 0)
		mockVenue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			for {
				msgType, data, err := conn.ReadMessage()
				if err != nil {
					return
				}

				receivedMessages = append(receivedMessages, string(data))

				// If discovery message received, respond with instrument snapshot
				if strings.Contains(string(data), "instrument") {
					resp := []byte(`{"channel":"instrument","type":"snapshot","data":{"pairs":["BTC/USD","ETH/USD","SOL/USD"]}}`)
					_ = conn.WriteMessage(msgType, resp)
				}
			}
		}))
		defer mockVenue.Close()

		wsURL := "ws://" + strings.TrimPrefix(mockVenue.URL, "http://")

		// 1. Initialize Circuit Nodes
		wsClient := websocket.NewWebSocketClient(ctx)
		capWS := websocket.WebSocketClient_ServerToClient(wsClient)

		condServer := NewCondition(ctx)
		capCond := Condition_ServerToClient(condServer)

		onceServer := NewOnce(ctx)
		capOnce := Once_ServerToClient(onceServer)

		jsonDiscovery := types.NewJSON(ctx)
		capJSON := types.JSON_ServerToClient(jsonDiscovery)

		matchServer := NewMatch(ctx)
		capMatch := Match_ServerToClient(matchServer)

		batchServer := NewBatch(ctx)
		capBatch := Batch_ServerToClient(batchServer)

		delayServer := NewDelay(ctx)
		capDelay := Delay_ServerToClient(delayServer)

		// 2. Configure Discovery JSON payload
		discoveryMsg := `{"method":"instrument","params":{}}`
		err := capJSON.Write(ctx, func(params types.JSON_write_Params) error {
			return params.SetText(discoveryMsg)
		})
		So(err, ShouldBeNil)

		jsonFuture, releaseJSON := capJSON.Done(ctx, nil)
		jsonRes, err := jsonFuture.Struct()
		So(err, ShouldBeNil)
		discBytes, err := jsonRes.Out()
		So(err, ShouldBeNil)
		discBytesCopy := append([]byte(nil), discBytes...)
		releaseJSON()

		// Feed JSON payload to Once.through
		err = capOnce.Write(ctx, func(params Once_write_Params) error {
			return params.SetThrough(discBytesCopy)
		})
		So(err, ShouldBeNil)

		// 3. Connect WebSocket Client to endpoint
		err = capWS.Write(ctx, func(params websocket.WebSocketClient_write_Params) error {
			return params.SetEndpoint(wsURL)
		})
		So(err, ShouldBeNil)

		// Wait briefly for connection
		time.Sleep(150 * time.Millisecond)
		So(wsClient.Status(), ShouldEqual, runtime.READY)

		// 4. WebSocket Status -> Condition -> Once Trigger
		wsFuture, releaseWS := capWS.Done(ctx, nil)
		wsRes, err := wsFuture.Struct()
		So(err, ShouldBeNil)
		wsStatus := wsRes.Status()
		releaseWS()

		err = capCond.Write(ctx, func(params Condition_write_Params) error {
			params.SetStatus(wsStatus)
			return nil
		})
		So(err, ShouldBeNil)

		condFuture, releaseCond := capCond.Done(ctx, nil)
		condRes, err := condFuture.Struct()
		So(err, ShouldBeNil)
		readyTrigger := condRes.Ready()
		releaseCond()
		So(readyTrigger, ShouldBeTrue)

		// 5. Condition.ready fires Once.trigger -> passes discovery JSON to WebSocketClient.write
		err = capOnce.Write(ctx, func(params Once_write_Params) error {
			params.SetTrigger(readyTrigger)
			return nil
		})
		So(err, ShouldBeNil)

		onceFuture, releaseOnce := capOnce.Done(ctx, nil)
		onceRes, err := onceFuture.Struct()
		So(err, ShouldBeNil)
		So(onceRes.Fired(), ShouldBeTrue)
		outPayload, err := onceRes.Out()
		So(err, ShouldBeNil)
		discPayload := append([]byte(nil), outPayload...)
		releaseOnce()

		err = capWS.Write(ctx, func(params websocket.WebSocketClient_write_Params) error {
			return params.SetWrite(discPayload)
		})
		So(err, ShouldBeNil)

		// Wait for mock venue response
		time.Sleep(150 * time.Millisecond)
		So(len(receivedMessages), ShouldBeGreaterThanOrEqualTo, 1)
		So(receivedMessages[0], ShouldContainSubstring, "instrument")

		// 6. WebSocketClient.read -> Match("instrument")
		wsFuture2, releaseWS2 := capWS.Done(ctx, nil)
		wsRes2, err := wsFuture2.Struct()
		So(err, ShouldBeNil)
		inboundFrame, err := wsRes2.Read()
		So(err, ShouldBeNil)
		inboundFrameCopy := append([]byte(nil), inboundFrame...)
		releaseWS2()

		err = capMatch.Write(ctx, func(params Match_write_Params) error {
			if err := params.SetPattern("instrument"); err != nil {
				return err
			}
			return params.SetData(inboundFrameCopy)
		})
		So(err, ShouldBeNil)

		matchFuture, releaseMatch := capMatch.Done(ctx, nil)
		matchRes, err := matchFuture.Struct()
		So(err, ShouldBeNil)
		So(matchRes.Matched(), ShouldBeTrue)
		matchedData, err := matchRes.Out()
		So(err, ShouldBeNil)
		So(string(matchedData), ShouldContainSubstring, "BTC/USD")
		releaseMatch()

		// 7. Matched response -> Batching -> Delay -> WebSocketClient.write
		subscriptionItems := []string{`"BTC/USD"`, `"ETH/USD"`, `"SOL/USD"`}
		for _, item := range subscriptionItems {
			err = capBatch.Write(ctx, func(params Batch_write_Params) error {
				params.SetSize(3)
				return params.SetItem([]byte(item))
			})
			So(err, ShouldBeNil)
		}

		batchFuture, releaseBatch := capBatch.Done(ctx, nil)
		batchRes, err := batchFuture.Struct()
		So(err, ShouldBeNil)
		So(batchRes.Ready(), ShouldBeTrue)
		batchData, err := batchRes.Out()
		So(err, ShouldBeNil)
		batchDataCopy := append([]byte(nil), batchData...)
		releaseBatch()

		// Delay line paces the batch
		err = capDelay.Write(ctx, func(params Delay_write_Params) error {
			params.SetMillis(25)
			return params.SetData(batchDataCopy)
		})
		So(err, ShouldBeNil)

		delayFuture, releaseDelay := capDelay.Done(ctx, nil)
		delayRes, err := delayFuture.Struct()
		So(err, ShouldBeNil)
		So(delayRes.Ready(), ShouldBeTrue)
		pacedBatch, err := delayRes.Out()
		So(err, ShouldBeNil)
		pacedBatchCopy := append([]byte(nil), pacedBatch...)
		releaseDelay()

		// Send batched subscriptions to WebSocket client
		err = capWS.Write(ctx, func(params websocket.WebSocketClient_write_Params) error {
			return params.SetWrite(pacedBatchCopy)
		})
		So(err, ShouldBeNil)

		time.Sleep(100 * time.Millisecond)
		So(len(receivedMessages), ShouldBeGreaterThanOrEqualTo, 2)
		So(receivedMessages[1], ShouldEqual, `["BTC/USD","ETH/USD","SOL/USD"]`)

		_ = wsClient.Close()
	})
}
