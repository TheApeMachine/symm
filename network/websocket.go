package network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue/kraken"
)

const readNotReadyDelay = 50 * time.Millisecond

/*
WebsocketClient is a reconnecting Kraken WS transport. Disconnects drop to
WAITING and redial with backoff — never System.Error, or every later Read/Write
floods "client is not ready".
*/
type WebsocketClient struct {
	*runtime.System

	dialer       *gorilla.Dialer
	conn         *gorilla.Conn
	rest         *REST
	url          string
	pinger       *Pinger
	writeMu      sync.Mutex
	reconnectMu  sync.Mutex
	callbackMu   sync.Mutex
	reconnecting bool
	maxBackoff   time.Duration
	callbacks    []func() error
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
	client.callbackMu.Lock()
	client.callbacks = append(client.callbacks, hook)
	client.callbackMu.Unlock()
}

/*
SetCallback replaces reconnect hooks (does not append). L3 recovery peels
symbols onto the same socket and must not stack duplicate subscribe handlers.
*/
func (client *WebsocketClient) SetCallback(hook func() error) {
	if client == nil {
		return
	}

	client.callbackMu.Lock()
	defer client.callbackMu.Unlock()

	if hook == nil {
		client.callbacks = nil
		return
	}

	client.callbacks = []func() error{hook}
}

func (client *WebsocketClient) Open() (err error) {
	if client.Context().Err() != nil {
		return errnie.Error(client.Context().Err())
	}

	client.writeMu.Lock()

	if client.conn != nil {
		_ = client.conn.Close()
		client.conn = nil
	}

	client.conn, _, err = client.dialer.Dial(client.url, nil)
	conn := client.conn
	client.writeMu.Unlock()

	if err != nil {
		client.Transition(runtime.ERROR)

		return errnie.Err(
			errnie.IO,
			"[network.websocket] error dialing url",
			err,
		)
	}

	status := client.Status()

	if client.pinger.Status() == runtime.WAITING {
		client.pinger.Start(conn, &client.writeMu, &status)
	} else {
		client.writeMu.Lock()
		client.pinger.conn = conn
		client.writeMu.Unlock()
	}

	client.Transition(runtime.READY)
	client.pinger.Transition(runtime.READY)

	client.callbackMu.Lock()
	hooks := append([]func() error(nil), client.callbacks...)
	client.callbackMu.Unlock()

	for _, callback := range hooks {
		if err := callback(); err != nil {
			client.abortOpen(conn)

			return errnie.Err(
				errnie.IO,
				"[network.websocket] reconnect hook failed",
				err,
			)
		}
	}

	return nil
}

func (client *WebsocketClient) abortOpen(conn *gorilla.Conn) {
	client.writeMu.Lock()

	if client.conn == conn {
		if client.conn != nil {
			_ = client.conn.Close()
		}

		client.conn = nil
	}

	client.pinger.conn = nil
	client.writeMu.Unlock()

	client.Transition(runtime.ERROR)
	client.pinger.Transition(runtime.ERROR)
}

func (client *WebsocketClient) Read(_ ...string) (msg []byte, err error) {
	for {
		if err := client.Context().Err(); err != nil {
			return nil, errnie.Error(err)
		}

		conn, ready := client.liveConn()

		if !ready {
			if !client.sleep(readNotReadyDelay) {
				return nil, errnie.Error(client.Context().Err())
			}

			continue
		}

		_, message, err := conn.ReadMessage()

		if err != nil {
			client.scheduleReconnect(err)

			return nil, errnie.Err(
				errnie.IO,
				"[network.websocket] read failed",
				err,
			)
		}

		return message, nil
	}
}

func (client *WebsocketClient) liveConn() (*gorilla.Conn, bool) {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()

	if client.conn == nil || client.Status() != runtime.READY {
		return nil, false
	}

	return client.conn, true
}

func (client *WebsocketClient) Write(message []byte) error {
	client.writeMu.Lock()
	conn := client.conn
	ready := conn != nil && client.Status() == runtime.READY

	if !ready {
		client.writeMu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.IO, "[network.websocket] not ready for write", nil,
		))
	}

	err := conn.WriteMessage(gorilla.TextMessage, message)
	client.writeMu.Unlock()

	if err != nil {
		client.scheduleReconnect(err)

		return errnie.Error(err)
	}

	return nil
}

func (client *WebsocketClient) scheduleReconnect(err error) {
	if err == nil {
		return
	}

	client.reconnectMu.Lock()

	if client.reconnecting {
		client.reconnectMu.Unlock()

		return
	}

	client.reconnecting = true
	client.reconnectMu.Unlock()

	go client.reconnectLoop()
}

func (client *WebsocketClient) reconnectLoop() {
	defer func() {
		client.reconnectMu.Lock()
		client.reconnecting = false
		client.reconnectMu.Unlock()
	}()

	client.Transition(runtime.ERROR)
	client.pinger.Transition(runtime.ERROR)

	client.writeMu.Lock()

	if client.conn != nil {
		_ = client.conn.Close()
		client.conn = nil
	}

	client.pinger.conn = nil
	client.writeMu.Unlock()

	backoff := time.Second

	for {
		if client.Context().Err() != nil {
			return
		}

		wait := min(backoff, client.maxBackoff)

		if cool := kraken.Private.Waiting(); cool > wait {
			wait = cool
		}

		if !client.sleep(wait) {
			return
		}

		if openErr := client.Open(); openErr == nil {
			return
		}

		backoff += time.Second
	}
}

// Reconnect starts a single background redial loop (idempotent).
func (client *WebsocketClient) Reconnect(err error) {
	client.scheduleReconnect(err)
}

func (client *WebsocketClient) Post(url string, body any) ([]byte, error) {
	response, err := client.rest.Post(url, body)

	if err != nil {
		wrapped := errnie.Err(
			errnie.IO, "[network.websocket] rest post failed", err,
		).With("url", url)

		// Watch already backs off; transient closes must not ERROR every poll.
		if restTransient(err) || Recoverable(err) {
			return nil, wrapped
		}

		return nil, errnie.Error(wrapped)
	}

	if response == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "[network.websocket] empty post response", nil,
		).With("url", url))
	}

	return response.Body(), nil
}

func (client *WebsocketClient) Get(url string, params map[string]string) ([]byte, error) {
	response, err := client.rest.Get(url, params)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "[network.websocket] rest get failed", err,
		).With("url", url))
	}

	if response == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "[network.websocket] empty get response", nil,
		).With("url", url))
	}

	return response.Body(), nil
}

/*
Watch polls url on an interval. body may be a value or func() map[string]any
/ func() map[string]string (ticker rotates pairs). Public Ticker is GET.
Throttle and rate-limit responses pause further polls. Backoff is local so
ticker REST does not change the websocket reconnect cap.
*/
func (client *WebsocketClient) Watch(
	url string, body any, dest json.Unmarshaler, after func(),
) {
	go func() {
		interval := 5 * time.Second
		maxBackoff := client.maxBackoff
		private := strings.Contains(url, "/private/")

		if private {
			interval = system.Cfg.WebSocket.PrivateRefresh

			if max := system.Cfg.WebSocket.PrivateBackoffMax; max > 0 {
				maxBackoff = max
			}
		}

		if strings.Contains(url, "/Ticker") {
			if refresh := system.Cfg.WebSocket.TickerRefresh; refresh > 0 {
				interval = refresh
			}

			if max := system.Cfg.WebSocket.TickerBackoffMax; max > 0 {
				maxBackoff = max
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		var backoff time.Duration

		if !client.pollWatch(url, body, dest, after, &backoff, maxBackoff) {
			return
		}

		for {
			select {
			case <-client.Context().Done():
				return
			case <-ticker.C:
				if !client.pollWatch(url, body, dest, after, &backoff, maxBackoff) {
					return
				}
			}
		}
	}()
}

func (client *WebsocketClient) pollWatch(
	url string,
	body any,
	dest json.Unmarshaler,
	after func(),
	backoff *time.Duration,
	maxBackoff time.Duration,
) bool {
	if client.Context().Err() != nil {
		return false
	}

	if *backoff > 0 {
		wait := *backoff

		if maxBackoff > 0 && wait > maxBackoff {
			wait = maxBackoff
		}

		if !client.sleep(wait) {
			return false
		}
	}

	raw, err := client.fetch(url, body)

	if err != nil {
		if cool := kraken.Private.Waiting(); cool > 0 {
			*backoff = cool
		} else if *backoff == 0 {
			*backoff = time.Second
		} else {
			*backoff *= 2
		}

		return client.Context().Err() == nil
	}

	if len(raw) == 0 {
		return true
	}

	if errs := kraken.VenueErrors(raw); len(errs) > 0 {
		// Private.Observe already ran under Post's Hold for private URLs.
		if until, ok := kraken.ThrottleUntil(errs...); ok {
			if wait := time.Until(until); wait > 0 {
				if maxBackoff > 0 && wait > maxBackoff {
					wait = maxBackoff
				}

				if !client.sleep(wait) {
					return false
				}
			}

			*backoff = 0

			return client.Context().Err() == nil
		}

		if kraken.RateLimited(errs...) {
			if cool := kraken.Private.Waiting(); cool > 0 {
				*backoff = cool
			} else {
				*backoff += time.Second
			}

			return client.Context().Err() == nil
		}
	}

	if err := dest.UnmarshalJSON(raw); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[network.websocket] unmarshal failed", err,
		).With("url", url))

		if *backoff < time.Second {
			*backoff = time.Second
		} else {
			*backoff += time.Second
		}

		if maxBackoff > 0 && *backoff > maxBackoff {
			*backoff = maxBackoff
		}

		return client.Context().Err() == nil
	}

	if after != nil {
		after()
	}

	*backoff = 0

	return true
}

func (client *WebsocketClient) fetch(url string, body any) ([]byte, error) {
	payload := watchPayload(body)

	if strings.Contains(url, "/0/public/Ticker") {
		params := stringParams(payload)

		if params["pair"] == "" {
			return nil, nil
		}

		return client.Get(url, params)
	}

	return client.Post(url, payload)
}

func (client *WebsocketClient) sleep(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-client.Context().Done():
		return false
	case <-timer.C:
		return true
	}
}

func watchPayload(body any) any {
	switch next := body.(type) {
	case func() map[string]any:
		return next()
	case func() map[string]string:
		return next()
	default:
		return body
	}
}

func stringParams(payload any) map[string]string {
	switch values := payload.(type) {
	case map[string]string:
		return values
	case map[string]any:
		out := make(map[string]string, len(values))

		for key, value := range values {
			if value == nil {
				continue
			}

			out[key] = fmt.Sprint(value)
		}

		return out
	default:
		return nil
	}
}

func (client *WebsocketClient) Close() error {
	client.Transition(runtime.ERROR)
	client.pinger.Transition(runtime.ERROR)
	client.pinger.Close()

	client.writeMu.Lock()

	if client.conn != nil {
		_ = client.conn.Close()
		client.conn = nil
	}

	client.writeMu.Unlock()

	return client.System.Close()
}