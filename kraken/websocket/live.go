package websocket

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"sync"
	"time"

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
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q[any]
	client      *spot.WebSocket
	rest        *spot.REST
	sync        *sync.Map
	paper       *Paper
	simulator   *Simulator
	url         string
	restURL     string
	auth        bool
	instruments bool
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
		rest:        spot.NewREST(),
		sync:        &sync.Map{},
		url:         baseURL,
		restURL:     restURL,
		auth:        auth,
		instruments: instruments,
	}

	if live.auth {
		live.client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")
		live.rest.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.rest.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

		if viper.GetString("trading.model") != "live" {
			live.simulator = LatencySimulator()
			live.paper = NewPaper(ctx, pool, baseURL, auth, live.simulator)
		}
	}

	if live.simulator == nil && viper.GetString("trading.model") != "live" {
		live.simulator = LatencySimulator()
	}

	live.client.OnReceived.Recurring(func(event *callback.Event[*kraken.WebSocketMessage]) {
		raw := event.Data.Bytes()
		channel := utils.GetString(raw, "channel")

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

		for _, callback := range value.([]func([]byte)) {
			callback(raw)
		}
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		if live.auth {
			errnie.Error(live.client.Authenticate())
			return
		}
	})

	errnie.Error(live.client.Connect())
	return live
}

func (live *Live) Client() *spot.WebSocket {
	return live.client
}

func (live *Live) Paper() *Paper {
	return live.paper
}

func (live *Live) On(
	channel string, action func([]byte),
) {
	callbacks, ok := live.sync.LoadOrStore(channel, []func([]byte){action})

	if ok {
		callbacks = append(callbacks.([]func([]byte)), action)
		live.sync.Store(channel, callbacks)
	}
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

	started := time.Now()

	writeErr := live.client.WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if live.simulator != nil {
		live.simulator.Record(WEBSOCKET, time.Since(started))
	}

	return errnie.Error(writeErr)
}

func (live *Live) do(options spot.RequestOptions) ([]byte, error) {
	started := time.Now()

	request := errnie.Does(func() (*kraken.Request, error) {
		return live.rest.NewRequest(options)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}).Value()

	resp := errnie.Does(func() (*kraken.Response, error) {
		return request.Do()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}).Value()

	if live.simulator != nil {
		live.simulator.Record(REST, time.Since(started))
	}

	return resp.Body, nil
}

func (live *Live) Get(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		Auth:       live.auth,
		Path:       path,
		Method:     "GET",
		Query:      params,
		PublicKey:  live.client.REST.PublicKey,
		PrivateKey: live.client.REST.PrivateKey,
	})
}

func (live *Live) Post(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		Auth:       live.auth,
		Path:       path,
		Method:     "POST",
		Body:       params,
		PublicKey:  live.client.REST.PublicKey,
		PrivateKey: live.client.REST.PrivateKey,
	})
}

func (live *Live) Close() {
	if live.paper != nil {
		live.paper.Close()
	}

	live.cancel()
	live.client.Disconnect()
}
