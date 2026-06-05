package paper

import (
	"container/ring"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/paper/response"
	"github.com/theapemachine/symm/kraken/paper/types"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

/*
WebSocket simulates Kraken private websocket traffic on raw and kraken:private.
*/
type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	err         error
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	sockets     map[string]types.Socket
	balances    *response.Balances
	orders      *response.Orders
	latencies   *ring.Ring
}

func NewWebSocket(ctx context.Context, pool *qpool.Q) *WebSocket {
	return NewWebSocketWithQuoteCache(ctx, pool, broker.EnsureQuoteCache(ctx, pool))
}

/*
NewWebSocketWithQuoteCache builds the paper socket with explicit quote state.
*/
func NewWebSocketWithQuoteCache(
	ctx context.Context,
	pool *qpool.Q,
	quotes *broker.QuoteCache,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	latencies, latencyErr := loadLatencyProfile()

	balances := response.NewBalances(pool.CreateBroadcastGroup("ui", 10*time.Millisecond))
	orders := response.NewOrdersWithQuoteCache(ctx, pool, balances, quotes)

	ws := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		sockets: map[string]types.Socket{
			"balances": balances,
			"orders":   orders,
		},
		balances:  balances,
		orders:    orders,
		latencies: latencies,
		err:       latencyErr,
	}

	for _, channel := range []string{"raw", "kraken:private"} {
		ws.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		ws.subscribers[channel] = ws.broadcasts[channel].Subscribe(
			"kraken/paper:"+channel, 1024,
		)
	}

	if err := user.NewBalance(pool, nil); err != nil {
		errnie.Error(err)
	}

	errnie.Info("kraken/paper websocket ready", "kraken/paper websocket")

	return ws
}

func (ws *WebSocket) Connect(
	endpoint public.EndpointType, channel string, n uint64,
) error {
	return ws.err
}

func (ws *WebSocket) Tick() (err error) {
	if ws.err != nil {
		return ws.err
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	publishedInitialWallet := false

	for {
		select {
		case <-ws.ctx.Done():
			return ws.err
		case message, ok := <-ws.subscribers["kraken:private"].Incoming:
			if !ok || message == nil {
				return ws.ctx.Err()
			}

			ws.broadcasts["raw"].Send(&qpool.QValue[any]{
				Type:  message.Type,
				Value: ws.sockets[message.Type].Send(message),
			})
		case <-ticker.C:
			// Poll resting protective orders against the latest quote — the paper
			// emulation of Kraken's server-side trigger engine.
			if !publishedInitialWallet {
				ws.balances.PublishUI()
				publishedInitialWallet = true
			}

			ws.orders.CheckTriggers()
			time.Sleep(ws.latencies.Value.(time.Duration))
			ws.latencies.Next()
		}
	}
}

func loadLatencyProfile() (*ring.Ring, error) {
	latencies := ring.New(64)

	for range 64 {
		latencies.Value = time.Duration(0)
		latencies = latencies.Next()
	}

	defaultLatency := viper.GetDuration("trading.paper.default_one_way_latency")

	if defaultLatency > 0 {
		for range 64 {
			latencies.Value = defaultLatency
			latencies = latencies.Next()
		}
	}

	path := viper.GetString("trading.paper.latency_profile")

	if path == "" {
		path = "runs/network_latency.json"
	}

	latencyFile, err := os.OpenFile(path, os.O_RDONLY, 0644)

	if err != nil {
		if defaultLatency > 0 {
			return latencies, nil
		}

		return latencies, fmt.Errorf("paper websocket latency profile %q: %w", path, err)
	}

	defer latencyFile.Close()

	for {
		var value time.Duration

		if _, err = fmt.Fscanf(latencyFile, "%d\n", &value); err != nil {
			break
		}

		latencies.Value = value
		latencies = latencies.Next()
	}

	return latencies, nil
}

func (ws *WebSocket) Close() error {
	ws.cancel()
	return nil
}
