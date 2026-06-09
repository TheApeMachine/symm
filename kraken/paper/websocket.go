package paper

import (
	"container/ring"
	"context"
	"errors"
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
	sockets     map[string]types.Socket
	balances    *response.Balances
	orders      *response.Orders
	executions  *response.Executions
	latencies   *ring.Ring
	isConnected atomic.Bool
}

func NewWebSocket(ctx context.Context, pool *qpool.Q[any]) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	ws := &WebSocket{
		ctx:    ctx,
		cancel: cancel,
		pool:   pool,
		bus: internal.NewBus(
			ctx,
			pool,
			[]string{"raw", "kraken:private", "ui"},
			[]string{"kraken:private"},
		),
		sockets: map[string]types.Socket{
			"balances":   response.NewBalances(ctx, pool),
			"orders":     response.NewOrders(ctx, pool),
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

	return ws
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
				"kraken:private",
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

			var response *types.SocketMessage

			switch message.Type {
			case "balances":
				response = ws.sockets["balances"].Send(message)
			case "orders":
				response = ws.sockets["orders"].Send(message)
			case "executions":
				response = ws.sockets["executions"].Send(message)
			default:
				qvaluePool.Put(message)
				continue
			}

			if response == nil {
				qvaluePool.Put(message)
				continue
			}

			ws.handleErrors(response)

			switch response.Channel {
			case "balances":
				balances := user.Balances{}

				if err := errnie.Error(response.Unmarshal(&balances)); err != nil {
					response.Release()
					qvaluePool.Put(message)
					continue
				}

				ws.bus.Send("raw", "balances", balances)
				ws.bus.Send("ui", "balances", balances)
			case "orders":
				orders := []trading.OrderUpdate{}

				if err := errnie.Error(response.Unmarshal(&orders)); err != nil {
					response.Release()
					qvaluePool.Put(message)
					continue
				}

				ws.bus.Send("raw", "orders", orders)
			case "executions":
				executions := []user.Execution{}

				if err := errnie.Error(response.Unmarshal(&executions)); err != nil {
					response.Release()
					qvaluePool.Put(message)
					continue
				}

				ws.bus.Send("raw", "executions", executions)
			}

			response.Release()
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
