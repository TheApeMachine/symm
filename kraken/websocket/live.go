package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	gorillawebsocket "github.com/gorilla/websocket"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	sdkkraken "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

const (
	PublicWebSocketURL  = "wss://ws.kraken.com/v2"
	PrivateWebSocketURL = "wss://ws-auth.kraken.com/v2"
	Level3WebSocketURL  = "wss://ws-l3.kraken.com/v2"
)

var entityMap = map[string]func([]byte) any{
	"ticker":     func(buf []byte) any { return kraken.NewTicker(buf) },
	"book":       func(buf []byte) any { return kraken.NewBook(buf) },
	"trade":      func(buf []byte) any { return kraken.NewTrade(buf) },
	"level3":     func(buf []byte) any { return kraken.NewLevel3(buf) },
	"instrument": func(buf []byte) any { return kraken.NewInstrument(buf) },
}

/*
Live is one spot websocket session: SDK client, channel fan-out, auth/nonce,
and Sub* resubscribe after the SDK reconnects.
*/
type Live struct {
	*types.Actor
	status       types.Status
	ctx          context.Context
	cancel       context.CancelFunc
	client       *spot.WebSocket
	simulator    *Simulator
	books        *spot.BookManager
	bookMu       sync.RWMutex
	isLevel3     bool
	symbols      []string
	auth         bool
	nonce        *AuthNonce
	nonceErr     error
	ready        func() error
	readyGate    *types.ReadyFuture
	level3Queue  chan []byte
	level3Ledger *level3Ledger
	roots        map[string]*types.Subscription[any]
	increments   Increments
}

/*
New opens a spot websocket session and wires SDK callbacks in the constructor.
*/
func New(
	ctx context.Context,
	simulator *Simulator,
	auth bool,
	endpoint string,
) *Live {
	ctx, cancel := context.WithCancel(ctx)

	live := &Live{
		ctx:       ctx,
		cancel:    cancel,
		status:    types.INITIALIZING,
		simulator: simulator,
		client:    spot.NewWebSocket(),
		auth:      auth,
		readyGate: types.NewReadyFuture(),
		roots: map[string]*types.Subscription[any]{
			"ticker":     types.NewSubscription[any](),
			"book":       types.NewSubscription[any](),
			"trade":      types.NewSubscription[any](),
			"level3":     types.NewSubscription[any](),
			"instrument": types.NewSubscription[any](),
			"balances":   types.NewSubscription[any](),
			"executions": types.NewSubscription[any](),
			"add_order":  types.NewSubscription[any](),
		},
	}

	live.Actor = types.NewActor(ctx, "live", nil)

	for name, root := range live.roots {
		live.AddRoot(name, root)
	}

	live.Actor.Initialize()
	live.client.URL = endpoint

	if auth {
		nonce, err := processAuthNonce()
		live.nonce = nonce
		live.nonceErr = err
		live.wireCredentials()
	}

	if endpoint == Level3WebSocketURL {
		live.isLevel3 = true
		live.configureLevel3()
	}

	live.client.OnReceived.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		raw := event.Data.Bytes()

		if live.isLevel3 && utils.GetString(raw, "channel") == "level3" {
			live.enqueueLevel3(raw)
		}

		channel := utils.GetString(raw, "channel")

		if channel == "" {
			if method := utils.GetString(raw, "method"); method == "add_order" {
				channel = method
			}
		}

		if channel == "" {
			if message := utils.GetString(raw, "error"); message != "" {
				errnie.Error(errnie.Err(errnie.Validation, message, nil))
			}

			return
		}

		if channel == "status" || channel == "heartbeat" || (live.isLevel3 && channel == "level3") {
			return
		}

		if channel == "balances" || channel == "executions" || channel == "add_order" {
			live.roots[channel].Send(raw)
			return
		}

		if entity, ok := entityMap[channel]; ok {
			entity := entity(raw)

			switch channel {
			case "instrument":
				live.increments.Remember(entity.(*kraken.Instrument))
			case "book":
				if errnie.Error(live.increments.Stamp(entity.(*kraken.Book))) != nil {
					return
				}
			}

			live.roots[channel].Send(entity)
		}
	})

	live.client.OnConnected.Recurring(func(event *callback.Event[any]) {
		live.onConnected()
	})

	if auth {
		live.client.OnAuthenticated.Recurring(func(event *callback.Event[string]) {
			live.onAuthenticated()
		})
	}

	return live
}

func (live *Live) Status() types.Status {
	return live.status
}

func (live *Live) wireCredentials() {
	if live.client == nil || !live.auth {
		return
	}

	live.client.REST.PublicKey = os.Getenv("KRAKEN_API_KEY")
	live.client.REST.PrivateKey = os.Getenv("KRAKEN_API_SECRET")

	if live.nonceErr != nil || live.nonce == nil {
		return
	}

	// Private and every Level3 batch authenticate with the same key; they
	// must share one monotonic nonce sequence or concurrent token fetches
	// collide (EAPI:Invalid nonce).
	live.client.REST.Nonce = live.nonce.Next
}

func (live *Live) onConnected() {
	if !live.auth {
		if live.ready != nil {
			if err := live.ready(); err != nil {
				live.status = types.ERROR
				live.readyGate.Resolve(err)
				errnie.Error(errnie.Err(
					errnie.Validation,
					"websocket: public resubscribe failed",
					err,
				))

				return
			}
		}

		live.status = types.READY
		live.readyGate.Resolve(nil)

		return
	}

	if errnie.Error(live.authenticate()) != nil {
		live.status = types.ERROR
		live.readyGate.Resolve(types.ClosedError{Component: "websocket:auth"})
	}
}

func (live *Live) onAuthenticated() {
	if live.isLevel3 && len(live.symbols) > 0 && live.SubscribeLevel3(
		live.symbols,
		viper.GetInt("market.l3_depth"),
	) != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: level3 book subscription failed",
			nil,
		))
		live.status = types.ERROR
		live.readyGate.Resolve(types.ClosedError{Component: "websocket:level3"})

		return
	}

	if live.ready != nil {
		if err := live.ready(); err != nil {
			live.status = types.ERROR
			live.readyGate.Resolve(err)
			errnie.Error(errnie.Err(
				errnie.Validation,
				"websocket: private resubscribe failed",
				err,
			))

			return
		}
	}

	live.status = types.READY
	live.readyGate.Resolve(nil)
}

func (live *Live) authenticate() (err error) {
	if live.nonceErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: auth nonce unavailable",
			live.nonceErr,
		))
	}

	if err = live.client.Authenticate(); err != nil && !strings.Contains(err.Error(), "Invalid nonce") {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: authentication failed",
			err,
		))
	}

	if err == nil {
		return nil
	}

	if live.nonce != nil {
		live.nonce.Bump()
	}

	return live.client.Authenticate()
}

/*
configureLevel3 installs BookManager + exact-text ledger and starts the
off-reader Level3 worker.
*/
func (live *Live) configureLevel3() {
	live.books = spot.NewBookManager()
	live.level3Ledger = newLevel3Ledger()
	live.books.OnCreateBook.Recurring(func(event *callback.Event[*book.Book]) {
		managed := event.Data
		managed.EnableMaxDepth = false
		managed.NoBookCrossing = false
	})

	live.client.OnSent.Recurring(func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
		live.ingestLevel3Sent(event)
	})

	live.level3Queue = make(chan []byte, level3QueueDepth)
	go live.drainLevel3()
}

func (live *Live) Initialize() error {
	errnie.Info("initializing live")

	if err := live.client.Connect(); err != nil {
		live.status = types.ERROR
		live.readyGate.Resolve(err)

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"websocket: connect failed",
			err,
		))
	}

	return nil
}

/*
Ready returns the future that resolves once auth and required subs complete.
*/
func (live *Live) Ready() *types.ReadyFuture {
	if live.readyGate == nil {
		live.readyGate = types.NewReadyFuture()
	}

	return live.readyGate
}

/*
Root returns the Actor fan-out for this session.
*/
func (live *Live) Root() *types.Actor {
	return live.Actor
}

func (live *Live) Client() *spot.WebSocket {
	return live.client
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

	started := time.Now()

	err = live.client.WriteMessage(
		gorillawebsocket.TextMessage, raw,
	)

	if live.simulator != nil {
		live.simulator.Record(WEBSOCKET, time.Since(started))
	}

	return errnie.Error(err)
}

func (live *Live) do(options spot.RequestOptions) ([]byte, error) {
	started := time.Now()

	request, err := live.client.REST.NewRequest(options)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}

	resp, err := request.Do()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"Kraken REST request failed",
			err,
		))
	}

	errors := utils.GetStringSlice(resp.Body, "error")

	if len(errors) > 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			errors[0],
			nil,
		))
	}

	if resp.StatusCode != 200 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"websocket.Live.do[%d]: %s",
				resp.StatusCode,
				resp.Body,
			),
			nil,
		))
	}

	if live.simulator != nil {
		live.simulator.Record(REST, time.Since(started))
	}

	return resp.Body, nil
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
	live.cancel()

	if live.client.IsActive() {
		errnie.Error(live.client.Disconnect())
	}
}
