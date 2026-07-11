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
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Live is the spot websocket and REST transport.
*/
type Live struct {
	status      types.Status
	ctx         context.Context
	cancel      context.CancelFunc
	client      *spot.WebSocket
	sync        *sync.Map
	paper       *Paper
	simulator   *Simulator
	auth        bool
	instruments bool
}

/*
New opens a spot websocket transport.
*/
func New(
	ctx context.Context,
	simulator *Simulator,
	auth bool,
	instruments bool,
) *Live {
	ctx, cancel := context.WithCancel(ctx)

	live := &Live{
		status:      types.INITIALIZING,
		ctx:         ctx,
		cancel:      cancel,
		simulator:   simulator,
		client:      spot.NewWebSocket(),
		sync:        &sync.Map{},
		auth:        auth,
		instruments: instruments,
	}

	if live.auth {
		live.client.URL = "wss://ws-auth.kraken.com/v2"
		live.client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
		live.client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

		// Kraken remembers the last accepted nonce per key across process
		// restarts. The vendored counter defaults to second granularity, so
		// any restart landing within the same wall-clock second as the prior
		// run's last request resets to a lower nonce and gets rejected.
		// Microsecond granularity keeps the restart-collision window well
		// below realistic process startup latency while staying inside the
		// int64 range Kraken expects for the nonce field.
		nonceCounter := kraken.NewEpochCounter()
		nonceCounter.Granularity = time.Microsecond
		live.client.REST.Nonce = nonceCounter.Get
	}

	if !live.auth {
		live.client.URL = "wss://ws.kraken.com/v2"
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
		if !live.auth {
			live.status = types.READY
			return
		}

		if errnie.Error(live.client.Authenticate()) != nil {
			live.status = types.ERROR
		}
	})

	if live.auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			live.status = types.READY
		})
	}

	if errnie.Error(live.client.Connect()) != nil {
		live.status = types.ERROR
		return live
	}

	if !live.auth {
		live.status = types.READY
	}

	return live
}

func (live *Live) Status() types.Status {
	return live.status
}

func (live *Live) Client() *spot.WebSocket {
	return live.client
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
		return live.client.REST.NewRequest(options)
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
		Auth:   live.auth,
		Path:   path,
		Method: "GET",
		Query:  params,
	})
}

func (live *Live) Post(
	path string, params json.Marshaler,
) ([]byte, error) {
	return live.do(spot.RequestOptions{
		Auth:   live.auth,
		Path:   path,
		Method: "POST",
		Body:   params,
	})
}

func (live *Live) Close() {
	if live.paper != nil {
		live.paper.Close()
	}

	live.cancel()
	live.client.Disconnect()
}
