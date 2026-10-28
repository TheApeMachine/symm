package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
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

	conn         *gorilla.Conn
	url          string
	pinger       *Pinger
	onReconnect  func() error
	onDisconnect func()
	backoff      time.Duration
	// hookBackoff grows with consecutive reconnect hook failures and is
	// cleared by a hook that succeeds. A dial that succeeds resets backoff,
	// so a hook that keeps failing behind good dials needs its own.
	hookBackoff time.Duration
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

A hook failure drops the connection: a redialed socket whose resubscribe
failed carries no data and would otherwise look healthy while the stream is
dark. The failure is logged and returned from Open/Read/Write, the client
waits in WAITING, and the next Read or Write redials with the doubled backoff
and runs the hook again, so a resubscribe that fails for a while (a token the
venue refuses until a nonce recovers) does not end the stream for good.
*/
func (client *WebsocketClient) OnReconnect(hook func() error) {
	client.onReconnect = hook
}

/*
OnDisconnect registers a hook invoked when a live connection drops (before
any redial). Consumers whose state is only valid while the stream is live
(a Level3 book) mark it stale here, so readers do not see frozen state for
the whole backoff window. Nil clears the hook.
*/
func (client *WebsocketClient) OnDisconnect(hook func()) {
	client.onDisconnect = hook
}

func (client *WebsocketClient) Open(url string) error {
	client.url = url
	err := client.dialLocked()
	hook := client.onReconnect

	if err != nil {
		return err
	}

	return client.runHook(hook)
}

/*
runHook runs the reconnect hook outside the client lock. A failure drops the
connection and grows the backoff so the next Read or Write redials and retries
the hook (see OnReconnect).
*/
func (client *WebsocketClient) runHook(hook func() error) error {
	if hook == nil {
		return nil
	}

	hookErr := hook()

	if hookErr == nil {
		client.hookBackoff = 0

		return nil
	}

	cause := errnie.Err(
		errnie.IO,
		"[network.websocket] reconnect hook failed",
		hookErr,
	)

	client.closeConnLocked()

	switch client.Status() {
	case runtime.READY, runtime.ERROR, runtime.BUSY:
		client.Transition(runtime.WAITING)
	}

	client.hookBackoff = min(max(2*client.hookBackoff, time.Second), 30*time.Second)

	errnie.Error(cause)

	return cause
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
func (client *WebsocketClient) Read(keys ...string) (msg []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = client.softDown(fmt.Errorf("[network.websocket] read panic: %v", r))
		}
	}()

	if err := client.ensureReady(); err != nil {
		return nil, err
	}

	conn := client.conn

	if conn == nil {
		return nil, client.softDown(errnie.Error(errnie.Err(
			errnie.IO,
			"[network.websocket] connection missing",
			nil,
		)))
	}

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return nil, client.softDown(errnie.Err(errnie.IO, "[network.websocket] read deadline reset failed", err))
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

	conn := client.conn

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
	conn := client.conn
	ready := client.Status() == runtime.READY

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
		return client.softDown(errnie.Err(errnie.IO, "[network.websocket] ping failed", err))
	}

	return nil
}

func (client *WebsocketClient) ensureReady() error {
	if err := client.Context().Err(); err != nil {
		return err
	}

	if client.Status() == runtime.READY && client.conn != nil {
		return nil
	}

	if client.url == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[network.websocket] not open",
			nil,
		))
	}

	wait := min(max(max(client.backoff, client.hookBackoff), time.Second), 30*time.Second)

	select {
	case <-client.Context().Done():
		return client.Context().Err()
	case <-time.After(wait):
	}

	if client.Status() == runtime.READY && client.conn != nil {
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
		return err
	}

	return client.runHook(hook)
}

/*
SoftDown drops the active connection and places the client in WAITING state
without canceling its context, so subsequent Read operations trigger redial
with backoff. It invokes the OnDisconnect hook if the connection was live.
*/
func (client *WebsocketClient) SoftDown(cause error) error {
	wasLive := client.conn != nil
	hook := client.onDisconnect

	client.closeConnLocked()

	switch client.Status() {
	case runtime.READY, runtime.ERROR, runtime.BUSY:
		client.Transition(runtime.WAITING)
	}

	client.hookBackoff = min(max(2*client.hookBackoff, time.Second), 30*time.Second)
	client.warnOnce("disconnected: " + cause.Error())

	if wasLive && hook != nil {
		hook()
	}

	return cause
}

func (client *WebsocketClient) softDown(cause error) error {
	return client.SoftDown(cause)
}

func (client *WebsocketClient) closeConnLocked() {
	if client.conn == nil {
		return
	}

	// Teardown of a socket that already failed reports net.ErrClosed; that
	// is the expected state, anything else is raised.
	if err := client.conn.SetReadDeadline(time.Now()); err != nil && !errors.Is(err, net.ErrClosed) {
		errnie.Error(errnie.Err(errnie.IO, "[network.websocket] read deadline on close failed", err))
	}

	if err := client.conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		errnie.Error(errnie.Err(errnie.IO, "[network.websocket] close failed", err))
	}
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
	client.closeConnLocked()
	return client.System.Close()
}
