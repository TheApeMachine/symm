package paper

import (
	"container/ring"
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
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
	latencies   *ring.Ring
}

func NewWebSocket(ctx context.Context, pool *qpool.Q) *WebSocket {
	ctx, cancel := context.WithCancel(ctx)

	// Load the latency ring from the file.
	latencyFile, err := os.OpenFile("runs/network_latency.json", os.O_RDONLY, 0644)

	if err != nil {
		cancel()
		errnie.Error(err)
		return nil
	}

	defer latencyFile.Close()
	ring := ring.New(64)

	for {
		var value time.Duration
		_, err = fmt.Fscanf(latencyFile, "%d\n", &value)

		ring.Value = time.Duration(value)
		ring.Next()

		if err != nil {
			break
		}
	}

	balances := response.NewBalances(pool.CreateBroadcastGroup("ui", 10*time.Millisecond))

	ws := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		sockets: map[string]types.Socket{
			"balances": balances,
			"orders":   response.NewOrders(ctx, pool, balances),
		},
		latencies: ring,
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
	rndConnDelay := rand.Intn(100)
	time.Sleep(time.Duration(rndConnDelay) * time.Millisecond)
	return nil
}

func (ws *WebSocket) Tick() (err error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

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
			time.Sleep(ws.latencies.Value.(time.Duration))
			ws.latencies.Next()
		}
	}
}

func (ws *WebSocket) Close() error {
	ws.cancel()
	return nil
}
