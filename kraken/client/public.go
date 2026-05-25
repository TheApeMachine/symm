package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/kraken/instrument"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/ohlc"
	"github.com/theapemachine/symm/kraken/trade"
)

/*
PublicClient maintains an unauthenticated Kraken WebSocket v2 session.
*/
type PublicClient struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	subscribers map[string]*qpool.Subscriber
	tick        *qpool.BroadcastGroup
	trade       *qpool.BroadcastGroup
	book        *qpool.BroadcastGroup
	symbols     *qpool.BroadcastGroup
	ohlc        *qpool.BroadcastGroup
	ui          *qpool.BroadcastGroup
	conn        *websocket.Conn
	url         string
	once        sync.Once
	subscribed  map[string]struct{}
	catalogAt   time.Time
	replay      bool
}

func NewPublicClient(
	ctx context.Context,
	pool *qpool.Q,
	url string,
) *PublicClient {
	ctx, cancel := context.WithCancel(ctx)

	publicClient := &PublicClient{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: make(map[string]*qpool.Subscriber),
		url:         url,
		subscribed:  make(map[string]struct{}),
		replay:      config.System.ReplayFile != "",
	}

	subscriptions := pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)
	publicClient.subscribers["subscriptions"] = subscriptions.Subscribe("public:subscriptions", 128)
	publicClient.tick = pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
	publicClient.trade = pool.CreateBroadcastGroup("trade", 10*time.Millisecond)
	publicClient.book = pool.CreateBroadcastGroup("book", 10*time.Millisecond)
	publicClient.symbols = pool.CreateBroadcastGroup("symbols", 10*time.Millisecond)
	publicClient.ohlc = pool.CreateBroadcastGroup("ohlc", 10*time.Millisecond)

	return publicClient
}

func (publicClient *PublicClient) Start() error {
	return publicClient.Connect()
}

func (publicClient *PublicClient) State() engine.State {
	return engine.READY
}

func (publicClient *PublicClient) Tick() error {
	select {
	case <-publicClient.ctx.Done():
		publicClient.cancel()
		return publicClient.ctx.Err()
	case msg := <-publicClient.subscribers["subscriptions"].Incoming:
		symbols, ok := msg.Value.([]string)

		if !ok {
			return errnie.Error(fmt.Errorf("invalid subscriptions message: %v", msg))
		}

		if publicClient.replay || publicClient.conn == nil {
			return nil
		}

		pending := make([]string, 0, len(symbols))

		for _, symbol := range symbols {
			if symbol == "" {
				continue
			}

			if _, seen := publicClient.subscribed[symbol]; seen {
				continue
			}

			publicClient.subscribed[symbol] = struct{}{}
			pending = append(pending, symbol)
		}

		if len(pending) == 0 {
			return nil
		}

		if err := publicClient.conn.WriteJSON(ohlc.NewSubscribe(pending)); err != nil {
			return errnie.Error(err)
		}

		if err := publicClient.conn.WriteJSON(trade.NewSubscribe(pending)); err != nil {
			return errnie.Error(err)
		}

		if err := publicClient.conn.WriteJSON(map[string]any{
			"method": "subscribe",
			"params": market.SubscribeParams{}.Ticker(pending),
		}); err != nil {
			return errnie.Error(err)
		}

		if err := publicClient.conn.WriteJSON(map[string]any{
			"method": "subscribe",
			"params": market.SubscribeParams{}.Book(
				pending, config.System.BookDepthLevels,
			),
		}); err != nil {
			return errnie.Error(err)
		}

		return nil
	default:
		return nil
	}
}

func (publicClient *PublicClient) Connect() error {
	publicClient.once.Do(func() {
		if publicClient.replay {
			go publicClient.runReplay()
			return
		}

		go publicClient.runLive()
	})

	return nil
}

func (publicClient *PublicClient) Close() error {
	publicClient.cancel()
	return nil
}

func (publicClient *PublicClient) runReplay() {
	file, err := os.Open(config.System.ReplayFile)

	if err != nil {
		errnie.Error(err)
		return
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	pace := config.System.ReplayPace

	for publicClient.ctx.Err() == nil && scanner.Scan() {
		line := scanner.Bytes()

		if len(line) == 0 {
			continue
		}

		publicClient.read(line)

		if pace > 0 {
			time.Sleep(pace)
		}
	}

	if err := scanner.Err(); err != nil {
		errnie.Error(err)
	}
}

func (publicClient *PublicClient) runLive() {
	backoff := 30 * time.Second

	for publicClient.ctx.Err() == nil {
		conn, _, err := websocket.DefaultDialer.DialContext(
			publicClient.ctx, publicClient.url, nil,
		)

		if err != nil {
			errnie.Error(err)

			select {
			case <-publicClient.ctx.Done():
				return
			case <-time.After(backoff):
			}

			continue
		}

		publicClient.conn = conn
		publicClient.subscribed = make(map[string]struct{})
		publicClient.catalogAt = time.Time{}

		if err := conn.WriteJSON(instrument.NewSubscribe()); err != nil {
			errnie.Error(err)
			conn.Close()
			publicClient.conn = nil

			select {
			case <-publicClient.ctx.Done():
				return
			case <-time.After(backoff):
			}

			continue
		}

		pingStop := make(chan struct{})

		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-publicClient.ctx.Done():
					return
				case <-pingStop:
					return
				case <-ticker.C:
					if err := conn.WriteJSON(map[string]string{"method": "ping"}); err != nil {
						return
					}
				}
			}
		}()

		for publicClient.ctx.Err() == nil {
			_, payload, err := conn.ReadMessage()

			if err != nil {
				errnie.Error(err)
				break
			}

			publicClient.read(payload)
		}

		close(pingStop)
		conn.Close()
		publicClient.conn = nil

		select {
		case <-publicClient.ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (publicClient *PublicClient) read(payload []byte) {
	channel, err := market.ChannelName(payload)

	if err != nil {
		return
	}

	switch channel {
	case core.ChannelInstrument:
		var message struct {
			Type string `json:"type"`
			Data struct {
				Pairs []instrument.Data `json:"pairs"`
			} `json:"data"`
		}

		if json.Unmarshal(payload, &message) != nil {
			return
		}

		if message.Type != "snapshot" && !publicClient.catalogAt.IsZero() {
			return
		}

		if !publicClient.catalogAt.IsZero() {
			refresh := 6 * time.Hour

			if time.Since(publicClient.catalogAt) < refresh {
				return
			}
		}

		pairs := make(map[string]*asset.Pair, len(message.Data.Pairs))

		for _, row := range message.Data.Pairs {
			if row.Status != string(market.PairStatusOnline) {
				continue
			}

			pair := row.Pair()
			pairs[row.Symbol] = &pair
		}

		if len(pairs) == 0 {
			return
		}

		publicClient.catalogAt = time.Now()
		publicClient.symbols.Send(&qpool.QValue[any]{Value: pairs})
		publicClient.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event": "quote_progress",
			"ts":    time.Now().UTC().Format(time.RFC3339Nano),
			"ready": 0,
			"total": len(pairs),
		}})
	case core.ChannelTicker:
		rows, err := market.ParseTickerRows(payload)

		if err != nil {
			return
		}

		for _, row := range rows {
			publicClient.tick.Send(&qpool.QValue[any]{Value: row})
		}
	case core.ChannelTrades, "trade":
		var message trade.Snapshot

		if json.Unmarshal(payload, &message) != nil {
			return
		}

		for _, row := range message.Data {
			publicClient.trade.Send(&qpool.QValue[any]{Value: row})
		}
	case core.ChannelOHLC:
		var message ohlc.Snapshot

		if json.Unmarshal(payload, &message) != nil {
			return
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)

		for _, row := range message.Data {
			publicClient.ohlc.Send(&qpool.QValue[any]{Value: row})
			publicClient.ui.Send(&qpool.QValue[any]{Value: map[string]any{
				"event":  "candle_bar",
				"ts":     now,
				"symbol": row.Symbol,
				"sec":    row.IntervalBegin.Unix(),
				"open":   row.Open,
				"high":   row.High,
				"low":    row.Low,
				"close":  row.Close,
				"volume": row.Volume,
			}})
		}
	case core.ChannelBook:
		delta, err := market.ParseBookLevelsDelta(payload)

		if err != nil {
			return
		}

		publicClient.book.Send(&qpool.QValue[any]{Value: delta})
	}
}
