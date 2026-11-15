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
	client.writeMu.Lock()
	client.conn, _, err = client.dialer.Dial(client.url, nil)
	conn := client.conn
	client.writeMu.Unlock()

	if err != nil {
		return errnie.Error(err)
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

	for _, callback := range client.callbacks {
		if err := callback(); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

func (client *WebsocketClient) Read(_ ...string) (msg []byte, err error) {
	client.writeMu.Lock()
	conn := client.conn
	ready := conn != nil && client.Status() == runtime.READY
	client.writeMu.Unlock()

	if !ready {
		return nil, nil
	}

	_, message, err := conn.ReadMessage()

	if err != nil {
		client.Reconnect(err)
	}

	return message, nil
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
		client.Reconnect(err)
		return errnie.Error(err)
	}

	return nil
}

func (client *WebsocketClient) Reconnect(err error) {
	if err == nil {
		return
	}

	backoff := time.Second

	client.Transition(runtime.ERROR)
	client.pinger.Transition(runtime.ERROR)

	client.writeMu.Lock()

	if client.conn != nil {
		_ = client.conn.Close()
		client.conn = nil
	}

	client.pinger.conn = nil
	client.writeMu.Unlock()

	for {
		time.Sleep(min(backoff, client.maxBackoff))

		if openErr := client.Open(); openErr == nil {
			return
		}

		backoff += time.Second
	}
}

func (client *WebsocketClient) Post(url string, body any) ([]byte, error) {
	response, err := client.rest.Post(url, body)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO, "[network.websocket] rest post failed", err,
		).With("url", url))
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
func (client *WebsocketClient) Watch(url string, body any, dest json.Unmarshaler) {
	go func() {
		interval := 5 * time.Second
		maxBackoff := client.maxBackoff

		if strings.Contains(url, "/private/") {
			interval = system.Cfg.WebSocket.PrivateRefresh
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

		if !client.pollWatch(url, body, dest, &backoff, maxBackoff) {
			return
		}

		for {
			select {
			case <-client.Context().Done():
				return
			case <-ticker.C:
				if !client.pollWatch(url, body, dest, &backoff, maxBackoff) {
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
		*backoff += time.Second

		return client.Context().Err() == nil
	}

	if len(raw) == 0 {
		return true
	}

	if errs := kraken.VenueErrors(raw); len(errs) > 0 {
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
			*backoff += time.Second

			return client.Context().Err() == nil
		}
	}

	if err := dest.UnmarshalJSON(raw); err != nil {
		errnie.Error(errnie.Err(
			errnie.IO, "[network.websocket] unmarshal failed", err,
		).With("url", url))

		return client.Context().Err() == nil
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
