package network

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
)

/*
WebsocketClient is a reconnecting Kraken WS transport. Disconnects drop to
WAITING and redial with backoff — never System.Error, or every later Read/Write
floods "client is not ready".
*/
type WebsocketClient struct {
	*runtime.System

	dialer     *gorilla.Dialer
	conn       *gorilla.Conn
	rest       *REST
	url        string
	pinger     *Pinger
	writeMu    sync.Mutex
	maxBackoff time.Duration
	callbacks  []func() error
}

func NewWebsocketClient(
	ctx context.Context,
	url string,
) *WebsocketClient {
	client := &WebsocketClient{
		url:        url,
		maxBackoff: 15 * time.Second,
		callbacks:  make([]func() error, 0),
		rest:       NewREST(),
		dialer: &gorilla.Dialer{
			HandshakeTimeout: 30 * time.Second,
		},
	}

	client.System = runtime.NewSystem(ctx, "network.websocket", client)

	client.pinger = NewPinger(ctx)
	client.pinger.Transition(runtime.WAITING)

	return client
}

func (client *WebsocketClient) Callback(hook func() error) {
	client.callbacks = append(client.callbacks, hook)
}

func (client *WebsocketClient) Open() (err error) {
	client.conn, _, err = client.dialer.Dial(client.url, nil)

	if err != nil {
		return errnie.Error(err)
	}

	status := client.Status()

	if client.pinger.Status() == runtime.WAITING {
		client.pinger.Start(client.conn, &client.writeMu, &status)
	}

	client.Transition(runtime.READY)
	client.pinger.Transition(runtime.READY)

	for _, callback := range client.callbacks {
		if err := callback(); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

func (client *WebsocketClient) Read(_ ...string) (msg []byte, err error) {
	if client.conn == nil || client.Status() != runtime.READY {
		return nil, nil
	}

	_, message, err := client.conn.ReadMessage()

	if err != nil {
		client.Reconnect(err)
	}

	return message, nil
}

func (client *WebsocketClient) Write(message []byte) error {
	if client.conn == nil || client.Status() != runtime.READY {
		return errnie.Error(errnie.Err(
			errnie.IO, "[network.websocket] not ready for write", nil,
		))
	}

	client.writeMu.Lock()
	err := client.conn.WriteMessage(gorilla.TextMessage, message)
	client.writeMu.Unlock()

	if err != nil {
		client.Reconnect(err)
	}

	return nil
}

func (client *WebsocketClient) Reconnect(err error) {
	if err == nil {
		return
	}

	backoff := time.Duration(1 * time.Second)

	client.Transition(runtime.ERROR)
	client.pinger.Transition(runtime.ERROR)

	client.conn.Close()

	for {
		time.Sleep(min(backoff, client.maxBackoff))
		err = client.Open()

		if err == nil {
			break
		}

		backoff = backoff + 1*time.Second
	}
}

func (client *WebsocketClient) Post(url string, body any) ([]byte, error) {
	response, err := client.rest.Post(url, body)

	if err != nil {
		return nil, errnie.Error(err)
	}

	if response == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "[network.websocket] empty post response", nil,
		))
	}

	return response.Body(), nil
}

/*
Watch polls url on an interval. body may be a value or func() map[string]any
(ticker rotates pairs). Throttle/rate-limit responses pause further polls.
*/
func (client *WebsocketClient) Watch(url string, body any, dest json.Unmarshaler) {
	go func() {
		interval := 5 * time.Second
		private := strings.Contains(url, "/private/")

		if private {
			interval = system.Cfg.WebSocket.PrivateRefresh
		}

		if strings.Contains(url, "/Ticker") {
			if refresh := system.Cfg.WebSocket.TickerRefresh; refresh > 0 {
				interval = refresh
			}

			if max := system.Cfg.WebSocket.TickerBackoffMax; max > 0 {
				client.maxBackoff = max
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		backoff := time.Duration(0)

		var (
			err error
			raw []byte
		)

		for {
			select {
			case <-client.Context().Done():
				return
			case <-ticker.C:
				if err != nil {
					backoff = backoff + 1*time.Second
					time.Sleep(min(backoff+1*time.Second, client.maxBackoff))
					err = nil
				}

				payload := body

				if next, ok := body.(func() map[string]any); ok {
					payload = next()
				}

				raw, err = client.Post(url, payload)

				if err != nil {
					errnie.Error(err)
					continue
				}

				if err := dest.UnmarshalJSON(raw); err != nil {
					errnie.Error(errnie.Err(
						errnie.IO, "[network.websocket] unmarshal failed", err,
					))

					continue
				}
			}
		}
	}()
}

func (client *WebsocketClient) Close() error {
	client.pinger.Close()
	client.conn.Close()
	return client.System.Close()
}
