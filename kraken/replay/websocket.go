package replay

import (
	"bufio"
	"context"
	"io"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/public"
)

/*
WebSocket replays lines from trading.replay.file. Orders go through paper.
*/
type WebSocket struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	paper       *paper.WebSocket
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	file        io.Reader
}

func NewWebSocket(ctx context.Context, pool *qpool.Q, file io.Reader) (*WebSocket, error) {
	ctx, cancel := context.WithCancel(ctx)

	paperSocket, err := paper.NewWebSocket(ctx, pool)

	if err != nil {
		cancel()
		return nil, err
	}

	ws := &WebSocket{
		ctx:         ctx,
		cancel:      cancel,
		paper:       paperSocket,
		pool:        pool,
		file:        file,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
	}

	for _, channel := range []string{
		"kraken:public", "ticker", "book", "trade", "ohlc", "instrument", "level3",
	} {
		ws.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		ws.subscribers[channel] = ws.broadcasts[channel].Subscribe(channel, 128)
	}

	return ws, errnie.Error(errnie.Require(map[string]any{
		"ctx":    ws.ctx,
		"cancel": ws.cancel,
		"paper":  ws.paper,
		"pool":   ws.pool,
		"file":   ws.file,
	}))
}

func (ws *WebSocket) Connect(endpoint public.EndpointType, channel string) error {
	return errnie.Error(ws.paper.Connect(endpoint, channel))
}

func (ws *WebSocket) Tick() error {
	scanner := bufio.NewScanner(ws.file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		select {
		case <-ws.ctx.Done():
			return ws.ctx.Err()
		default:
		}

		var message public.SocketMessage

		if err := sonic.Unmarshal(scanner.Bytes(), &message); err != nil {
			return err
		}

		ws.broadcasts[message.Channel].Send(&qpool.QValue[any]{Value: message})
	}

	return errnie.Error(scanner.Err())
}

func (ws *WebSocket) Close() error {
	ws.cancel()
	return nil
}
