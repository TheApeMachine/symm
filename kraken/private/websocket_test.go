package private

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	wswebsocket "github.com/fasthttp/websocket"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/types"
)

type staticTokenRest struct{}

func (staticTokenRest) WebSocketToken(
	ctx context.Context,
	token *types.Token,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	token.Token = "live-token"
	token.Expires = 900

	return nil
}

func TestWebSocketAddsTokenToPrivateRequest(t *testing.T) {
	types.BindTokenRest(staticTokenRest{})

	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	socket := NewWebSocket(ctx, pool, dmt.NewTree(""))
	payload := []byte(`{"method":"add_order","params":{"symbol":"BTC/USD","side":"buy","order_qty":0.1}}`)

	out, err := socket.payloadWithToken(payload)

	if err != nil {
		t.Fatal(err)
	}

	var envelope struct {
		Params map[string]any `json:"params"`
	}

	if err := sonic.Unmarshal(out, &envelope); err != nil {
		t.Fatal(err)
	}

	if envelope.Params["token"] != "live-token" {
		t.Fatalf("token = %v, want live-token", envelope.Params["token"])
	}
}

func TestWebSocketPublishesLiveBalancesLikePrivateBus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree("")
	subscription := pool.Subscribe("ui", nil)
	socket := NewWebSocket(ctx, pool, tree)
	wire := []byte(`{"channel":"balances","type":"snapshot","data":[{"asset":"USD","asset_class":"currency","balance":200}]}`)
	message := types.Acquire()
	defer message.Release()

	if err := message.Decode(wire); err != nil {
		t.Fatal(err)
	}

	if err := socket.publish(wire, message); err != nil {
		t.Fatal(err)
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()

	artifact, err := subscription.Wait(waitCtx)

	if err != nil {
		t.Fatal(err)
	}

	role, roleErr := artifact.Role()
	scope, scopeErr := artifact.Scope()

	if roleErr != nil {
		t.Fatal(roleErr)
	}

	if scopeErr != nil {
		t.Fatal(scopeErr)
	}

	if role != "balances" || scope != "snapshot" {
		t.Fatalf("role/scope = %q/%q, want balances/snapshot", role, scope)
	}

	var payload map[string]any

	if err := sonic.Unmarshal(artifact.DecryptPayload(), &payload); err != nil {
		t.Fatal(err)
	}

	rows, ok := payload["asset"].([]any)

	if !ok || len(rows) != 1 {
		t.Fatalf("asset rows = %#v, want one row", payload["asset"])
	}
}

func TestConnectHandlesNilHTTPResponse(t *testing.T) {
	types.BindTokenRest(staticTokenRest{})

	originalDial := dialWebSocket
	dialWebSocket = func(string, http.Header) (*wswebsocket.Conn, *http.Response, error) {
		return nil, nil, errors.New("dial failed before response")
	}
	t.Cleanup(func() {
		dialWebSocket = originalDial
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	socket := NewWebSocket(ctx, pool, dmt.NewTree(""))
	defer socket.Close()

	err := socket.Connect("ws://127.0.0.1:1", 1)

	if err == nil {
		t.Fatal("connect should fail when context expires")
	}
	if socket.isConnected.Load() {
		t.Fatal("socket should not be marked connected")
	}
}

func TestConnectRetriesAfterInitialFailure(t *testing.T) {
	types.BindTokenRest(staticTokenRest{})
	viper.Set("market.ws_reconnect_initial", time.Millisecond)
	viper.Set("market.ws_reconnect_max", time.Millisecond)
	viper.Set("market.ws_reconnect_multiplier", 1)
	t.Cleanup(viper.Reset)

	upgrader := wswebsocket.Upgrader{
		CheckOrigin: func(request *http.Request) bool {
			return true
		},
	}

	server := httptest.NewServer(http.HandlerFunc(
		func(response http.ResponseWriter, request *http.Request) {
			conn, err := upgrader.Upgrade(response, request, nil)
			if err != nil {
				t.Errorf("upgrade failed: %v", err)
				return
			}
			defer conn.Close()

			<-request.Context().Done()
		},
	))
	defer server.Close()

	originalDial := dialWebSocket
	attempts := 0
	dialWebSocket = func(endpoint string, header http.Header) (*wswebsocket.Conn, *http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, nil, errors.New("temporary dial failure")
		}
		return originalDial(endpoint, header)
	}
	t.Cleanup(func() {
		dialWebSocket = originalDial
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	socket := NewWebSocket(ctx, pool, dmt.NewTree(""))
	defer socket.Close()

	endpoint := "ws" + strings.TrimPrefix(server.URL, "http")

	if err := socket.Connect(endpoint, 1); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if attempts < 2 {
		t.Fatalf("attempts = %d, want retry", attempts)
	}
	if !socket.isConnected.Load() {
		t.Fatal("socket should be marked connected")
	}
}

func BenchmarkWebSocketPayloadWithToken(b *testing.B) {
	types.BindTokenRest(staticTokenRest{})

	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	socket := NewWebSocket(ctx, pool, dmt.NewTree(""))
	payload := []byte(`{"method":"add_order","params":{"symbol":"BTC/USD","side":"buy","order_qty":0.1}}`)

	b.ReportAllocs()

	for index := 0; index < b.N; index++ {
		if _, err := socket.payloadWithToken(payload); err != nil {
			b.Fatal(err)
		}
	}
}
