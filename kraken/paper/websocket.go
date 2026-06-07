package paper

import (
	"container/ring"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
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
	pool        *qpool.Q[any]
	err         error
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.BroadcastConsumer
	sockets     map[string]types.Socket
	balances    *response.Balances
	orders      *response.Orders
	executions  *response.Executions
	latencies   *ring.Ring
}

func NewWebSocket(ctx context.Context, pool *qpool.Q[any]) *WebSocket {
	return NewWebSocketWithQuoteCache(ctx, pool, broker.EnsureQuoteCache(ctx, pool))
}

/*
NewWebSocketWithQuoteCache builds the paper socket with explicit quote state.
*/
func NewWebSocketWithQuoteCache(
	ctx context.Context,
	pool *qpool.Q[any],
	quotes *broker.QuoteCache,
) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	latencies, latencyErr := loadLatencyProfile()

	raw := pool.CreateBroadcastGroup("raw", 10*time.Millisecond)
	ui := pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	ids := response.NewIdentifier()
	catalog := response.NewPairCatalog(ctx)

	balances := response.NewBalances(raw, ui, ids)
	executions := response.NewExecutions(raw, balances, ids)
	orders := response.NewOrdersWithQuoteCache(
		ctx, pool, balances, ids, quotes, catalog, newRingLatency(latencies),
		broker.EnsureStressCache(ctx, pool),
	)
	orders.Observe(executions)

	go catalog.Load()

	ws := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.BroadcastConsumer),
		sockets: map[string]types.Socket{
			"balances":   balances,
			"orders":     orders,
			"executions": executions,
		},
		balances:   balances,
		orders:     orders,
		executions: executions,
		latencies:  latencies,
		err:        latencyErr,
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

	if err := user.NewExecution(pool, nil); err != nil {
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
	private := ws.subscribers["kraken:private"]

	for {
		select {
		case <-ws.ctx.Done():
			return ws.err
		case <-ticker.C:
			if !publishedInitialWallet {
				ws.balances.PublishUI()
				publishedInitialWallet = true
			}

			ws.orders.CheckPending()
			ws.orders.CheckTriggers()

			latency, _ := ws.latencies.Value.(time.Duration)
			if latency > 0 {
				time.Sleep(latency)
			}

			ws.latencies = ws.latencies.Next()
		default:
			message := private.Poll()

			if message == nil {
				select {
				case <-ws.ctx.Done():
					return ws.err
				case <-ticker.C:
					if !publishedInitialWallet {
						ws.balances.PublishUI()
						publishedInitialWallet = true
					}

					ws.orders.CheckPending()
					ws.orders.CheckTriggers()

					latency, _ := ws.latencies.Value.(time.Duration)
					if latency > 0 {
						time.Sleep(latency)
					}

					ws.latencies = ws.latencies.Next()
				case <-time.After(2 * time.Millisecond):
				}

				continue
			}

			socket, ok := ws.sockets[message.Type]

			if !ok {
				continue
			}

			ws.broadcasts["raw"].Send(&qpool.QValue[any]{
				Type:  message.Type,
				Value: socket.Send(message),
			})
		}
	}
}

func loadLatencyProfile() (*ring.Ring, error) {
	latencies := ring.New(64)
	fillLatencyRing(latencies, nil)

	defaultLatency := viper.GetDuration("trading.paper.default_one_way_latency")

	if defaultLatency > 0 {
		fillLatencyRing(latencies, []time.Duration{defaultLatency})
	}

	path := strings.TrimSpace(viper.GetString("trading.paper.latency_profile"))

	if path == "" {
		path = "runs/network_latency.json"
	}

	raw, err := os.ReadFile(path)

	if err != nil {
		// The latency profile is observational, not a dependency of paper trading.
		// Missing files should not prevent the simulator from booting.
		return latencies, nil
	}

	profile := parseLatencyProfile(raw)

	if len(profile) > 0 {
		fillLatencyRing(latencies, profile)
	}

	return latencies, nil
}

func fillLatencyRing(latencies *ring.Ring, values []time.Duration) {
	if latencies == nil {
		return
	}

	if len(values) == 0 {
		values = []time.Duration{0}
	}

	cursor := latencies

	for index := 0; index < latencies.Len(); index++ {
		cursor.Value = values[index%len(values)]
		cursor = cursor.Next()
	}
}

func parseLatencyProfile(raw []byte) []time.Duration {
	var numeric []int64

	if err := json.Unmarshal(raw, &numeric); err == nil && len(numeric) > 0 {
		out := make([]time.Duration, 0, len(numeric))

		for _, value := range numeric {
			if value >= 0 {
				out = append(out, time.Duration(value))
			}
		}

		return out
	}

	fields := strings.Fields(string(raw))
	out := make([]time.Duration, 0, len(fields))

	for _, field := range fields {
		value, err := strconv.ParseInt(field, 10, 64)

		if err != nil || value < 0 {
			continue
		}

		out = append(out, time.Duration(value))
	}

	return out
}

func (ws *WebSocket) Close() error {
	ws.cancel()
	return nil
}
