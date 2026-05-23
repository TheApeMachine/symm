package kraken

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/symm/kraken/core"
)

type Websocket struct {
	ctx        context.Context
	cancel     context.CancelFunc
	conn       *websocket.Conn
	publicKey  string
	privateKey string
	token      *Token
	reqID      int
	mu         sync.Mutex
}

func NewWebsocket() *Websocket {
	ctx, cancel := context.WithCancel(context.Background())
	return &Websocket{ctx: ctx, cancel: cancel}
}

func NewAuthenticatedWebsocket(publicKey, privateKey string) *Websocket {
	ws := NewWebsocket()
	ws.publicKey = publicKey
	ws.privateKey = privateKey
	return ws
}

func (ws *Websocket) Connect() error {
	log.Printf("connecting to %s", core.KRAKEN_WS_URL)

	conn, _, err := websocket.DefaultDialer.DialContext(ws.ctx, core.KRAKEN_WS_URL, nil)
	if err != nil {
		return fmt.Errorf("dial websocket: %w", err)
	}

	ws.conn = conn

	if ws.publicKey != "" && ws.privateKey != "" {
		if err := ws.Authenticate(); err != nil {
			_ = ws.conn.Close()
			ws.conn = nil
			return fmt.Errorf("authenticate: %w", err)
		}
	}

	return nil
}

func (ws *Websocket) Authenticate() error {
	if ws.publicKey == "" || ws.privateKey == "" {
		return fmt.Errorf("websocket is not configured with API credentials")
	}

	token, err := NewToken(ws.publicKey, ws.privateKey)
	if err != nil {
		return err
	}

	ws.token = token
	return nil
}

func (ws *Websocket) NextReqID() int {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.reqID++
	return ws.reqID
}

func (ws *Websocket) Send(message any) error {
	if ws.conn == nil {
		return fmt.Errorf("websocket is not connected")
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	return ws.conn.WriteMessage(websocket.TextMessage, data)
}

func (ws *Websocket) Subscribe(sub *Subscription) error {
	if sub == nil {
		return fmt.Errorf("subscription is nil")
	}

	params := sub.Params
	if sub.RequiresAuth() {
		token, err := ws.tokenValue()
		if err != nil {
			return err
		}

		params, err = withToken(params, token)
		if err != nil {
			return err
		}
	}

	return ws.Send(&Subscription{
		Method: sub.Method,
		Params: params,
		ReqID:  sub.ReqID,
	})
}

func (ws *Websocket) SubscribeTo(params any, opts ...SubscriptionOption) error {
	opts = append(opts, WithReqID(ws.NextReqID()))
	return ws.Subscribe(NewSubscribe(params, opts...))
}

func (ws *Websocket) UnsubscribeFrom(params any, opts ...SubscriptionOption) error {
	opts = append(opts, WithReqID(ws.NextReqID()))
	return ws.Subscribe(NewUnsubscribe(params, opts...))
}

func (ws *Websocket) Close() error {
	ws.cancel()
	if ws.conn == nil {
		return nil
	}
	err := ws.conn.Close()
	ws.conn = nil
	return err
}

func (ws *Websocket) tokenValue() (string, error) {
	if ws.token == nil || ws.token.Expired() {
		if err := ws.Authenticate(); err != nil {
			return "", err
		}
	}
	return ws.token.Value(), nil
}
