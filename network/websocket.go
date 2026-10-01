package network

import (
	"context"
	"net/http"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
WebsocketClient is a reconnecting Kraken WS transport. Disconnects (1006,
connection reset, ping failure) drop to WAITING and redial with backoff.
They must not call System.Error: that floods ERROR on every later Read/Write
while status != READY ("client is not ready").
*/
type WebsocketClient struct {
	*runtime.System
	mu          sync.Mutex
	conn        *gorilla.Conn
	url         string
	pinger      *Pinger
	onReconnect func() error
	backoff     time.Duration
	lastWarn    time.Time
	dial        func(url string) (*gorilla.Conn, *http.Response, error)
}

func NewWebsocketClient(ctx context.Context) *WebsocketClient {
	client := &WebsocketClient{
		backoff: time.Second,
		dial: func(url string) (*gorilla.Conn, *http.Response, error) {
			return gorilla.DefaultDialer.Dial(url, nil)
		},
	}
	client.System = runtime.NewSystem(ctx, "network.websocket", client)
	client.pinger = NewPinger(ctx, client)
	return client
}

/*
OnReconnect registers a hook invoked after a successful redial (level3
resubscribe). Nil clears the hook.
*/
func (client *WebsocketClient) OnReconnect(hook func() error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.onReconnect = hook
}

func (client *WebsocketClient) Open(url string) error {
	client.mu.Lock()
	client.url = url
	err := client.dialLocked()
	hook := client.onReconnect
	client.mu.Unlock()

	if err != nil {
		return err
	}

	if hook != nil {
		if hookErr := hook(); hookErr != nil {
			client.mu.Lock()
			client.warnOnce("reconnect hook failed: " + hookErr.Error())
			client.mu.Unlock()
		}
	}

	return nil
}

func (client *WebsocketClient) dialLocked() error {
	if client.url == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[network.websocket] url is required",
			nil,
		))
	}

	client.closeConnLocked()

	conn, _, err := client.dial(client.url)

	if err != nil {
		client.Transition(runtime.WAITING)
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] error dialing url",
			err,
		))
	}

	client.conn = conn
	client.backoff = time.Second
	client.Transition(runtime.READY)
	client.pinger.Start()
	return nil
}

/*
Read returns the next text frame. On disconnect it drops to WAITING and
returns a soft error so ingress can retry without System.Error floods.
*/
func (client *WebsocketClient) Read() ([]byte, error) {
	if err := client.ensureReady(); err != nil {
		return nil, err
	}

	client.mu.Lock()
	conn := client.conn
	client.mu.Unlock()

	if conn == nil {
		return nil, client.softDown(errnie.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] connection missing",
			nil,
		)))
	}

	_, message, err := conn.ReadMessage()

	if err != nil {
		return nil, client.softDown(err)
	}

	return message, nil
}

/*
Write sends a text frame. Same soft-disconnect policy as Read.
*/
func (client *WebsocketClient) Write(message []byte) error {
	if err := client.ensureReady(); err != nil {
		return err
	}

	client.mu.Lock()
	conn := client.conn
	client.mu.Unlock()

	if conn == nil {
		return client.softDown(errnie.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] connection missing",
			nil,
		)))
	}

	err := conn.WriteMessage(gorilla.TextMessage, message)

	if err != nil {
		return client.softDown(err)
	}

	return nil
}

func (client *WebsocketClient) writePing() error {
	client.mu.Lock()
	conn := client.conn
	ready := client.Status() == runtime.READY
	client.mu.Unlock()

	if !ready || conn == nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] not ready for ping",
			nil,
		))
	}

	err := conn.WriteControl(
		gorilla.PingMessage,
		nil,
		time.Now().Add(5*time.Second),
	)

	if err != nil {
		_ = client.softDown(err)
		return err
	}

	return nil
}

func (client *WebsocketClient) ensureReady() error {
	if err := client.Context().Err(); err != nil {
		return err
	}

	client.mu.Lock()
	if client.Status() == runtime.READY && client.conn != nil {
		client.mu.Unlock()
		return nil
	}

	if client.url == "" {
		client.mu.Unlock()
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[network.websocket] not open",
			nil,
		))
	}

	wait := client.backoff
	if wait < time.Second {
		wait = time.Second
	}
	if wait > 30*time.Second {
		wait = 30 * time.Second
	}

	url := client.url
	_ = url
	client.mu.Unlock()

	select {
	case <-client.Context().Done():
		return client.Context().Err()
	case <-time.After(wait):
	}

	client.mu.Lock()
	if client.Status() == runtime.READY && client.conn != nil {
		client.mu.Unlock()
		return nil
	}

	client.warnOnce("reconnecting after disconnect")
	err := client.dialLocked()
	hook := client.onReconnect

	if err != nil {
		if client.backoff < 30*time.Second {
			client.backoff *= 2
			if client.backoff > 30*time.Second {
				client.backoff = 30 * time.Second
			}
		}
		client.mu.Unlock()
		return err
	}

	client.mu.Unlock()

	if hook != nil {
		if hookErr := hook(); hookErr != nil {
			client.mu.Lock()
			client.warnOnce("reconnect hook failed: " + hookErr.Error())
			client.mu.Unlock()
		}
	}

	return nil
}

func (client *WebsocketClient) softDown(cause error) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	client.closeConnLocked()

	switch client.Status() {
	case runtime.READY, runtime.ERROR, runtime.BUSY:
		client.Transition(runtime.WAITING)
	}

	client.warnOnce("disconnected: " + cause.Error())
	return cause
}

func (client *WebsocketClient) closeConnLocked() {
	if client.conn == nil {
		return
	}

	_ = client.conn.Close()
	client.conn = nil
	client.pinger.Stop()
}

func (client *WebsocketClient) warnOnce(message string) {
	now := time.Now()
	if now.Sub(client.lastWarn) < 5*time.Second {
		return
	}
	client.lastWarn = now
	errnie.Warn("[network.websocket] " + message)
}

/*
Close shuts the socket and cancels the client context.
*/
func (client *WebsocketClient) Close() error {
	client.mu.Lock()
	client.closeConnLocked()
	client.mu.Unlock()
	return client.System.Close()
}
