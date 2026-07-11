package websocket

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/bytedance/sonic"
	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/utils"
)

/*
Live is the spot websocket and REST transport.
*/
type Live struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	client        *spot.WebSocket
	sync          *sync.Map
	paper         *Paper
	url           string
	restURL       string
	auth          bool
	instruments   bool
	privateLock   bool
	authenticated atomic.Bool
	subscribed    *sync.Map
}

/*
New opens a spot websocket transport.
*/
func New(
	ctx context.Context,
	pool *qpool.Q[any],
	baseURL string,
	restURL string,
	auth bool,
	instruments bool,
) *Live {
	ctx, cancel := context.WithCancel(ctx)

	live := &Live{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		client:      spot.NewWebSocket(),
		sync:        &sync.Map{},
		url:         baseURL,
		restURL:     restURL,
		auth:        auth,
		instruments: instruments,
		privateLock: auth && strings.ToLower(strings.TrimSpace(
			viper.GetString("trading.model"),
		)) == "live",
		subscribed: &sync.Map{},
	}
	endpoint := strings.TrimSpace(baseURL)

	if !strings.HasPrefix(endpoint, "wss://") {
		endpoint = "wss://" + endpoint
	}

	live.client.WebSocket.URL = endpoint
	live.client.REST.BaseURL = strings.TrimRight(restURL, "/")

	model := strings.ToLower(strings.TrimSpace(viper.GetString("trading.model")))

	if live.auth && model == "paper" &&
		strings.Contains(endpoint, "ws-auth.kraken.com") {
		live.paper = NewPaper(ctx, pool, baseURL, auth)
	}

	if live.auth {
		live.client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")
	}

	live.client.OnReceived.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		raw := event.Data.Bytes()
		channel := utils.GetString(raw, "channel")

		method := utils.GetString(raw, "method")

		if channel == "" {
			channel = method
		}

		if channel == "" || slices.Contains(
			[]string{"status", "heartbeat"},
			channel,
		) {
			return
		}

		value, ok := live.sync.Load(channel)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: channel "+channel+" not found",
				nil,
			))

			return
		}

		var payload []byte
		var err error

		if channel == "book" || method != "" || live.auth {
			payload = raw
		} else {
			payload, err = utils.GetBytes(raw, "data")

			if err != nil {
				errnie.Error(err)
				return
			}
		}

		for _, callback := range value.([]func([]byte)) {
			callback(payload)
		}
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		if live.auth {
			errnie.Error(live.client.Authenticate())
			return
		}
	})

	live.client.OnAuthenticated.Recurring(func(
		event *callback.Event[string],
	) {
		live.authenticated.Store(true)
		live.sync.Range(func(channel, _ any) bool {
			errnie.Error(live.subscribePrivate(channel.(string)))
			return true
		})
	})

	errnie.Error(live.client.Connect())
	return live
}

func (live *Live) Client() *spot.WebSocket {
	return live.client
}

func (live *Live) On(
	channel string, action func([]byte),
) {
	if live.paper != nil {
		if _, ok := paperChannels[channel]; ok {
			live.paper.On(channel, action)
			return
		}
	}

	callbacks, ok := live.sync.LoadOrStore(channel, []func([]byte){action})

	if ok {
		callbacks = append(callbacks.([]func([]byte)), action)
		live.sync.Store(channel, callbacks)
	}

	if live.authenticated.Load() {
		errnie.Error(live.subscribePrivate(channel))
	}

}

func (live *Live) subscribePrivate(channel string) error {
	if !live.auth || live.paper != nil {
		return nil
	}

	if channel != "balances" && channel != "executions" {
		return nil
	}

	if _, loaded := live.subscribed.LoadOrStore(channel, struct{}{}); loaded {
		return nil
	}

	var err error

	if channel == "balances" {
		err = live.client.SubBalances()
	} else {
		err = live.client.SubExecutions()
	}

	if err != nil {
		live.subscribed.Delete(channel)
	}

	return err
}

func (live *Live) Write(params json.Marshaler) error {
	raw, err := params.MarshalJSON()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: write marshal failed",
			err,
		))
	}

	methodNode, err := sonic.Get(raw, "method")

	if err != nil || !methodNode.Exists() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	method, err := methodNode.String()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	if live.paper != nil && slices.Contains(
		[]string{"add_order", "cancel_order"},
		method,
	) {
		return live.paper.Write(params)
	}

	if live.privateLock && slices.Contains(
		[]string{"add_order", "cancel_order"},
		method,
	) {
		return errnie.Err(
			errnie.NotAcceptable,
			"live trading remediation lock: "+method+" is disabled",
			nil,
		)
	}

	return errnie.Error(live.client.WriteMessage(
		gorillawebsocket.TextMessage, raw,
	))
}

func (live *Live) do(options spot.RequestOptions) ([]byte, error) {
	options.Auth = live.auth
	response, err := live.client.REST.Call(options)

	if err != nil {
		return nil, errnie.Err(errnie.IO, "Kraken REST request failed", err)
	}

	if err := response.GetError(); err != nil {
		return nil, errnie.Err(errnie.Validation, "Kraken REST rejected request", err)
	}

	return sonic.Marshal(response.Result)
}

func (live *Live) Get(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		BaseURL: live.restURL,
		Path:    path,
		Method:  "GET",
		Query:   params,
	})
}

func (live *Live) Post(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		BaseURL: live.restURL,
		Path:    path,
		Method:  "POST",
		Body:    params,
	})
}

func (live *Live) Close() {
	if live.paper != nil {
		live.paper.Close()
	}

	live.cancel()
	live.client.Disconnect()
}
