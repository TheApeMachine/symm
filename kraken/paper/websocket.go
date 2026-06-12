package paper

import (
	"bufio"
	"container/ring"
	"context"
	"errors"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/kraken/paper/response"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/types"
	"github.com/theapemachine/symm/kraken/user"
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

	ws := &WebSocket{
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
		wsPingInterval: wsPingInterval,
		bus: internal.NewBus(
			ctx,
			pool,
			[]internal.Channel{internal.ChannelRaw, internal.ChannelKrakenPrivate, internal.ChannelUI},
			[]internal.Subscription{
				internal.Subscribe(internal.ChannelKrakenPrivate, "kraken:paper"),
			},
		),
		sockets: map[string]types.Socket{
			"balances":   response.NewBalances(ctx, pool, catalog),
			"orders":     response.NewOrders(ctx, pool, catalog),
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
	endpoint public.EndpointType, n uint64,
) error {
	if ws.isConnected.Load() {
		return nil
	}

	ws.emulateLatency()

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

func (ws *WebSocket) disconnect() {
	ws.isConnected.Store(false)
	ws.err = errors.New("simulated network disconnect")
}

var qvaluePool = sync.Pool{
	New: func() any {
		return &qpool.QValue[any]{}
	},
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

			if rand.Intn(10) == 0 {
				ws.disconnect()
			}
		}
	}
}

func (ws *WebSocket) read() {
	go func() {
		for {
			slot := qvaluePool.Get().(*qpool.QValue[any])

			var message *qpool.QValue[any]

			if message, ws.err = ws.bus.Receive(
				internal.ChannelKrakenPrivate,
			); errnie.Error(ws.err) != nil || message == nil {
				qvaluePool.Put(slot)
				break
			}

			qvaluePool.Put(slot)

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

				rawbus.Send(ws.bus, rawbus.TypeBalances, balances)
				ws.bus.Send(internal.ChannelUI, "balances", balances)
			case "orders":
				orders := []trading.OrderUpdate{}

				if err := errnie.Error(socketMessage.Unmarshal(&orders)); err != nil {
					socketMessage.Release()
					qvaluePool.Put(message)
					continue
				}

				rawbus.Send(ws.bus, rawbus.TypeOrders, orders)
			case "executions":
				executions := []user.Execution{}

				if err := errnie.Error(socketMessage.Unmarshal(&executions)); err != nil {
					socketMessage.Release()
					qvaluePool.Put(message)
					continue
				}

				rawbus.Send(ws.bus, rawbus.TypeExecutions, executions)
			}

			socketMessage.Release()
			qvaluePool.Put(message)
		}
	}()
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

func (ws *WebSocket) emulateLatency() {
	latency := ws.latencies.Value.(time.Duration)
	ws.latencies = ws.latencies.Next()
	time.Sleep(latency)
}

func (ws *WebSocket) loadLatencyProfile() (*ring.Ring, error) {
	profilePath := viper.GetString("trading.paper.latency_profile")

	if profilePath == "" {
		profilePath = "runs/network_latency.json"
	}

	profileBytes, err := os.ReadFile(profilePath)

	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultLatencyRing(), nil
		}

		return nil, errnie.Error(err)
	}

	samples := make([]time.Duration, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(profileBytes)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			return nil, errnie.Error(errors.New("paper websocket: empty latency profile"))
		}

		nanoseconds, err := strconv.ParseInt(line, 10, 64)

		if errnie.Error(err) != nil || nanoseconds <= 0 {
			return nil, errnie.Error(errors.New("paper websocket: invalid latency profile"))
		}

		samples = append(samples, time.Duration(nanoseconds))
	}

	if errnie.Error(scanner.Err()) != nil {
		return nil, scanner.Err()
	}

	if len(samples) == 0 {
		return nil, errnie.Error(errors.New("paper websocket: empty latency profile"))
	}

	ring := ring.New(len(samples))

	for _, sample := range samples {
		ring.Value = sample
		ring = ring.Next()
	}

	return ring, nil
}

func defaultLatencyRing() *ring.Ring {
	latencyRing := ring.New(8)
	defaultLatency := viper.GetDuration("trading.paper.default_latency")

	if defaultLatency <= 0 {
		defaultLatency = 25 * time.Millisecond
	}

	for range 8 {
		latencyRing.Value = defaultLatency
		latencyRing = latencyRing.Next()
	}

	return latencyRing
}
