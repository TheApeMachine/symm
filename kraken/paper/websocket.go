package paper

import (
	"container/ring"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/paper/response"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
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
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	bus         *internal.Bus
	bookStore   *krakenmarket.BookStore
	sockets     map[string]types.Socket
	balances    *response.Balances
	orders      *response.Orders
	executions  *response.Executions
	latencies   *ring.Ring
	isConnected atomic.Bool
}

func NewWebSocket(
	ctx context.Context,
	pool *qpool.Q[any],
	bookStore *krakenmarket.BookStore,
	catalog *response.PairCatalog,
) (*WebSocket, error) {
	if catalog == nil {
		return nil, fmt.Errorf("paper websocket: nil pair catalog")
	}

	ctx, cancel := context.WithCancel(ctx)

	balances, balancesErr := response.NewBalances(ctx, pool, catalog)

	if balancesErr != nil {
		cancel()

		return nil, balancesErr
	}

	ws := &WebSocket{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		bookStore: bookStore,
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelKrakenPrivate, internal.ChannelUI},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelKrakenPrivate, "kraken:paper"),
			},
		),
		sockets: map[string]types.Socket{
			"balances":   balances,
			"orders":     response.NewOrders(ctx, pool, bookStore),
			"executions": response.NewExecutions(ctx, pool),
		},
		isConnected: atomic.Bool{},
	}

	ws.sockets["orders"].Observe(
		ws.sockets["balances"],
	)

	ws.sockets["executions"].Observe(
		ws.sockets["balances"],
	)

	return ws, nil
}

/*
Connect using a simulated network delay. We can do a simple random delay to
model the network latency in this case. For individual requests, we should
use the latency profile, recorded from the actual Kraken API.
*/
func (ws *WebSocket) Connect(
	endpoint public.EndpointType, n uint64,
) error {
	if ws.isConnected.Load() {
		return nil
	}

	time.Sleep(
		time.Duration(
			rand.Intn(3)+rand.Intn(1000),
		) * time.Millisecond,
	)

	// Fail the connection randomly 10% of the time.
	if rand.Intn(10) == 0 {
		ws.err = errors.New("simulated network error")

		// Fibonacci gives us a good exponential backoff.
		n = uint64(
			math.Round((math.Pow(
				math.Phi, float64(n),
			) + math.Pow(
				math.Phi-1, float64(n),
			)) / math.Sqrt(5)),
		)

		time.Sleep(time.Duration(n) * time.Second)
		return ws.Connect(endpoint, n)
	}

	ws.isConnected.Store(true)
	return ws.err
}

var qvaluePool = sync.Pool{
	New: func() any {
		return &qpool.QValue[any]{}
	},
}

func (ws *WebSocket) Tick() (err error) {
	ticker := time.NewTicker(
		viper.GetDuration("market.ws_ping_interval"),
	)
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
			// Simulate the ping message.
			time.Sleep(
				time.Duration(
					rand.Intn(3)+rand.Intn(100),
				) * time.Millisecond,
			)
		default:
			slot := qvaluePool.Get().(*qpool.QValue[any])

			var message *qpool.QValue[any]

			if message, ws.err = ws.bus.Poll(
				internal.ChannelKrakenPrivate,
			); errnie.Error(ws.err) != nil || message == nil {
				qvaluePool.Put(slot)
				break
			}

			qvaluePool.Put(slot)

			// TODO: Use latency profile.
			time.Sleep(
				time.Duration(
					rand.Intn(3)+rand.Intn(100),
				) * time.Millisecond,
			)

			var socketMessage *types.SocketMessage

			switch message.Type {
			case "balances":
				socketMessage = ws.sockets["balances"].Send(message)
			case "orders":
				if frame, frameOK := message.Value.(types.KrakenMessage); frameOK &&
					frame.Method == trading.MethodAddOrder {
					params, paramsOK := addPaperParams(frame.Params)

					if paramsOK {
						if transitErr := trading.RejectStaleEntry(&params); transitErr != nil {
							errnie.Error(transitErr)
							qvaluePool.Put(message)
							continue
						}
					}
				}

				socketMessage = ws.sockets["orders"].Send(message)

				if orderSocket, ok := ws.sockets["orders"].(*response.Orders); ok {
					for _, execution := range orderSocket.DrainExecutions() {
						ws.bus.Send(internal.ChannelRaw, "executions", []user.Execution{execution})
					}

					ws.bus.Send(internal.ChannelRaw, "balances", orderSocket.Wallet())
					ws.bus.Send(internal.ChannelUI, "balances", orderSocket.Wallet())
				}
			case "executions":
				socketMessage = ws.sockets["executions"].Send(message)
			default:
				qvaluePool.Put(message)
				continue
			}

			if socketMessage == nil {
				qvaluePool.Put(message)
				continue
			}

			ws.handleErrors(socketMessage)

			switch socketMessage.Channel {
			case "balances":
				balances := user.Balances{}

				if err := errnie.Error(socketMessage.Unmarshal(&balances)); err != nil {
					socketMessage.Release()
					qvaluePool.Put(message)
					continue
				}

				ws.bus.Send(internal.ChannelRaw, "balances", balances)
				ws.bus.Send(internal.ChannelUI, "balances", balances)
			case "orders":
				orders := []trading.OrderUpdate{}

				if err := errnie.Error(socketMessage.Unmarshal(&orders)); err != nil {
					socketMessage.Release()
					qvaluePool.Put(message)
					continue
				}

				ws.bus.Send(internal.ChannelRaw, "orders", orders)
			case "executions":
				executions := []user.Execution{}

				if err := errnie.Error(socketMessage.Unmarshal(&executions)); err != nil {
					socketMessage.Release()
					qvaluePool.Put(message)
					continue
				}

				ws.bus.Send(internal.ChannelRaw, "executions", executions)
			}

			socketMessage.Release()
			qvaluePool.Put(message)
		}
	}
}

func (ws *WebSocket) handleErrors(message *types.SocketMessage) {
	for _, err := range message.Errors {
		switch strings.Split(err, ":")[0] {
		case "EOrder":
			time.Sleep(1 * time.Second)
		case "EService":
			unixTimestamp, err := strconv.ParseInt(strings.Split(err, ":")[1], 10, 64)

			if errnie.Error(err) != nil {
				continue
			}

			time.Sleep(time.Until(time.Unix(unixTimestamp, 0)))
		default:
			errnie.Error(errors.New(err))
		}
	}
}

func (ws *WebSocket) Close() error {
	ws.isConnected.Store(false)
	ws.cancel()

	return nil
}

func addPaperParams(value any) (trading.AddParams, bool) {
	switch typed := value.(type) {
	case trading.AddParams:
		return typed, true
	case *trading.AddParams:
		if typed == nil {
			return trading.AddParams{}, false
		}

		return *typed, true
	default:
		return trading.AddParams{}, false
	}
}
