package network

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"syscall"
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

	conn       *gorilla.Conn
	rest       *REST
	url        string
	pinger     *Pinger
	writeMu    sync.Mutex
	dialMu     sync.Mutex
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
	}

	client.System = runtime.NewSystem(ctx, "network.websocket", client)
	client.pinger = NewPinger(ctx)
	return client
}

func (client *WebsocketClient) Callback(hook func() error) {
	client.callbacks = append(client.callbacks, hook)
}

func (client *WebsocketClient) Open() (err error) {
	client.conn, _, err = gorilla.DefaultDialer.Dial(client.url, nil)

	if err != nil {
		return errnie.Error(err)
	}

	status := client.Status()
	client.pinger.Start(client.conn, &client.writeMu, &status)

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
		backoff := time.Duration(0)

		if errors.Is(
			err, io.EOF,
		) || errors.Is(
			err, gorilla.ErrReadLimit,
		) {
			client.conn.Close()
			client.Transition(runtime.WAITING)

			for err := client.Open(); err != nil; {
				time.Sleep(min(backoff+1*time.Second, client.maxBackoff))
			}

			return client.Read()
		}

		return nil, errnie.Error(err)
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

	backoff := time.Duration(0)

	if err != nil {
		if errors.Is(
			err, syscall.EPIPE,
		) || errors.Is(
			err, syscall.ECONNRESET,
		) || errors.Is(
			err, syscall.ECONNREFUSED,
		) || errors.Is(
			err, gorilla.ErrCloseSent,
		) || errors.Is(
			err, io.EOF,
		) {
			client.conn.Close()
			client.Transition(runtime.WAITING)

			for err := client.Open(); err != nil; {
				time.Sleep(backoff)
				backoff = min(backoff+1*time.Second, client.maxBackoff)
			}

			client.Write(message)
			return nil
		}

		return errnie.Error(err)
	}

	return nil
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

			raw, err = client.Post(url, body)

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
}

func (client *WebsocketClient) Close() error {
	client.pinger.Close()
	client.conn.Close()
	return client.System.Close()
}
