package paper

import (
	"container/ring"
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/paper/response"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/kraken/wsutil"
	"github.com/theapemachine/symm/observability"
	"github.com/theapemachine/symm/rawbus"
)

var baseURL = "wss://symm.kraken.com/v1/ws"

/*
WebSocket simulates the Kraken private websocket connection. This should be the
only point where the code makes any distinction between paper and live trading.
The idea is to have this be an emulation that mimics the live trading experience
as closely as possible. By doing this at the connection level, we can ensure that
all other code will work without any surprises once we switch to live trading.
*/
type WebSocket struct {
	ctx            context.Context
	cancel         context.CancelFunc
	err            error
	pool           *qpool.Q[any]
	bus            *internal.Bus
	sockets        map[string]types.Socket
	latencies      *ring.Ring
	failures       *paperFailureInjection
	isConnected    atomic.Bool
	wsPingInterval time.Duration
}

func NewWebSocket(
	ctx context.Context, pool *qpool.Q[any],
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)
	catalog := response.NewPairCatalog(ctx)
	marketConfig, _ := config.LoadMarketConfig()
	wsPingInterval := time.Second

	if marketConfig.WSPingInterval > 0 {
		wsPingInterval = marketConfig.WSPingInterval
	}

	orders, ordersErr := response.NewOrders(ctx, pool, catalog)

	if ordersErr != nil {
		cancel()
		return nil
	}

	failures, failureErr := newPaperFailureInjectionFromConfig()

	if failureErr != nil {
		cancel()
		return nil
	}

	ws := &WebSocket{
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
		wsPingInterval: wsPingInterval,
		failures:       failures,
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelKrakenPrivate, internal.ChannelUI},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelRaw, "kraken:paper:marks"),
				internal.Subscribe(internal.ChannelKrakenPrivate, "kraken:paper"),
			},
		),
		sockets: map[string]types.Socket{
			"balances":   response.NewBalances(ctx, pool, catalog),
			"orders":     orders,
			"executions": response.NewExecutions(ctx, pool),
		},
		isConnected: atomic.Bool{},
	}

	ws.latencies, ws.err = ws.loadLatencyProfile()

	if errnie.Error(ws.err) != nil {
		return nil
	}

	ws.sockets["orders"].Observe(
		ws.sockets["balances"],
	)

	ws.sockets["executions"].Observe(
		ws.sockets["balances"],
	)

	return ws
}

/*
Connect using a simulated network delay. We can do a simple random delay to
model the network latency in this case. For individual requests, we should
use the latency profile, recorded from the actual Kraken API.
*/
func (ws *WebSocket) Connect(
	endpoint public.EndpointType, attempt uint64,
) error {
	if ws.isConnected.Load() {
		return nil
	}

	ws.emulateLatency()

	if ws.failures.ConnectFailed() {
		ws.err = errors.New("paper websocket: simulated network error")
		observability.Shared().RecordWebSocketReconnect(
			"kraken/paper",
			string(endpoint),
			ws.err.Error(),
			time.Now().UTC(),
		)
		return ws.err
	}

	ws.err = nil
	ws.isConnected.Store(true)
	observability.Shared().RecordWebSocketConnected(
		"kraken/paper",
		string(endpoint),
		time.Now().UTC(),
	)
	return nil
}

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)
	ws.err = errors.New("simulated network disconnect")
	observability.Shared().RecordWebSocketReconnect(
		"kraken/paper",
		baseURL,
		ws.err.Error(),
		time.Now().UTC(),
	)
}

func (ws *WebSocket) Tick() (err error) {
	ws.read()

	ticker := time.NewTicker(ws.wsPingInterval)
	defer ticker.Stop()

	for {
		if ws.err = errnie.Error(ws.Connect(
			public.EndpointType(baseURL), 0,
		)); ws.err != nil {
			continue
		}

		select {
		case <-ws.ctx.Done():
			return ws.err
		case <-ticker.C:
			ws.emulateLatency()

			if ws.failures.Disconnected() {
				ws.disconnect()
			}
		}
	}
}

func (ws *WebSocket) read() {
	ws.readMarketMarks()

	go func() {
		for {
			var message *qpool.QValue[any]

			if message, ws.err = ws.bus.Receive(
				internal.ChannelKrakenPrivate,
			); errnie.Error(ws.err) != nil || message == nil {
				break
			}

			ws.emulateLatency()

			var socketMessage *types.SocketMessage

			switch message.Type {
			case "balances":
				socketMessage = ws.sockets["balances"].Send(message)
			case "orders":
				socketMessage = ws.sockets["orders"].Send(message)
			case "executions":
				socketMessage = ws.sockets["executions"].Send(message)
			default:
				continue
			}

			if socketMessage == nil {
				continue
			}

			ws.handleErrors(socketMessage)

			switch socketMessage.Channel {
			case "balances":
				balances := user.Balances{}

				if err := errnie.Error(socketMessage.Unmarshal(&balances)); err != nil {
					socketMessage.Release()
					continue
				}

				rawbus.Send(ws.bus, rawbus.TypeBalances, balances)
				ws.bus.Send(internal.ChannelUI, "balances", balances)
			case "orders":
				orders := []trading.OrderUpdate{}

				if err := errnie.Error(socketMessage.Unmarshal(&orders)); err != nil {
					socketMessage.Release()
					continue
				}

				rawbus.Send(ws.bus, rawbus.TypeOrders, orders)
				ws.publishPaperFills()
			case "executions":
				executions := []user.Execution{}

				if err := errnie.Error(socketMessage.Unmarshal(&executions)); err != nil {
					socketMessage.Release()
					continue
				}

				rawbus.Send(ws.bus, rawbus.TypeExecutions, executions)
			}

			socketMessage.Release()
		}
	}()
}

type tickerMarkUpdater interface {
	UpdateTicker(*market.TickerUpdate) bool
	Wallet() user.Balances
}

func (ws *WebSocket) readMarketMarks() {
	go func() {
		for {
			message, receiveErr := ws.bus.Receive(internal.ChannelRaw)

			if errnie.Error(receiveErr) != nil || message == nil {
				return
			}

			if rawbus.TypeFrom(message.Type) != rawbus.TypeTicker {
				continue
			}

			ws.publishTickerMarks(message)
		}
	}()
}

func (ws *WebSocket) publishTickerMarks(message *qpool.QValue[any]) {
	updates, ok := message.Value.(*market.TickerUpdates)

	if !ok || updates == nil {
		return
	}

	balances, ok := ws.sockets["balances"].(tickerMarkUpdater)

	if !ok || balances == nil {
		return
	}

	changed := false

	for _, ticker := range *updates {
		if balances.UpdateTicker(ticker) {
			changed = true
		}
	}

	if !changed {
		return
	}

	wallet := balances.Wallet()
	rawbus.Send(ws.bus, rawbus.TypeBalances, wallet)
	ws.bus.Send(internal.ChannelUI, "balances", wallet)
}

type paperFillDrain interface {
	DrainExecutions() []user.Execution
	Wallet() user.Balances
}

func (ws *WebSocket) publishPaperFills() {
	orders, ok := ws.sockets["orders"].(paperFillDrain)

	if !ok || orders == nil {
		return
	}

	executions := orders.DrainExecutions()

	if len(executions) == 0 {
		return
	}

	wallet := orders.Wallet()

	rawbus.Send(ws.bus, rawbus.TypeExecutions, executions)
	rawbus.Send(ws.bus, rawbus.TypeBalances, wallet)
	ws.bus.Send(internal.ChannelUI, "balances", wallet)
}

func (ws *WebSocket) handleErrors(message *types.SocketMessage) {
	for _, errorText := range message.Errors {
		exchangeError := wsutil.ParseExchangeError(errorText)
		decision := wsutil.DefaultExchangeErrorPolicy().Classify(exchangeError)
		observability.Shared().RecordExchangeError(
			"kraken/paper",
			exchangeError.Category,
			exchangeError.Code,
			string(decision.Action),
			exchangeError.Message,
			time.Now().UTC(),
		)

		handleErr := wsutil.HandleExchangePolicy(ws.ctx, exchangeError, decision)

		if handleErr == nil {
			continue
		}

		if internal.IsShutdown(handleErr) {
			return
		}

		errnie.Error(handleErr)
	}
}

func (ws *WebSocket) Close() error {
	ws.isConnected.Store(false)
	ws.cancel()

	return nil
}
