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
	"github.com/theapemachine/symm/wallet"
)

/*
PublicClient maintains an unauthenticated Kraken WebSocket v2 session.
*/
type PublicClient struct {
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
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
	heldSymbols map[string]struct{}
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
		heldSymbols: make(map[string]struct{}),
		replay:      config.System.ReplayFile != "",
	}

	subscriptions := pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)
	publicClient.subscribers["subscriptions"] = subscriptions.Subscribe("subscriptions", 128)
	publicClient.subscribers["wallet"] = pool.CreateBroadcastGroup("wallet", 10*time.Millisecond).
		Subscribe("wallet", 128)
	publicClient.tick = pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
	publicClient.trade = pool.CreateBroadcastGroup("trade", 10*time.Millisecond)
	publicClient.book = pool.CreateBroadcastGroup("book", 10*time.Millisecond)
	publicClient.symbols = pool.CreateBroadcastGroup("symbols", 10*time.Millisecond)
	publicClient.ohlc = pool.CreateBroadcastGroup("ohlc", 10*time.Millisecond)
	publicClient.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	return publicClient
}

func (publicClient *PublicClient) Start() error {
	return publicClient.Connect()
}

func (publicClient *PublicClient) State() engine.State {
	return engine.READY
}

func (publicClient *PublicClient) Tick() error {
	errnie.Info("starting public client tick")

	var workers sync.WaitGroup
	errs := make(chan error, 1)
	fail := func(err error) {
		select {
		case errs <- err:
			publicClient.cancel()
		default:
		}
	}

	workers.Go(func() {
		for {
			select {
			case <-publicClient.ctx.Done():
				return
			case msg, ok := <-publicClient.subscribers["subscriptions"].Incoming:
				if !ok {
					fail(fmt.Errorf("public subscriptions channel closed"))
					return
				}

				symbols, ok := msg.Value.([]string)

				if !ok {
					fail(fmt.Errorf("invalid subscriptions message: %v", msg))
					return
				}

				if err := publicClient.subscribeSymbols(symbols); err != nil {
					fail(err)
					return
				}
			}
		}
	})

	workers.Go(func() {
		for {
			select {
			case <-publicClient.ctx.Done():
				return
			case msg, ok := <-publicClient.subscribers["wallet"].Incoming:
				if !ok {
					fail(fmt.Errorf("public wallet channel closed"))
					return
				}

				wallet, ok := msg.Value.(*wallet.Wallet)

				if !ok || wallet == nil {
					fail(fmt.Errorf("invalid wallet message: %v", msg.Value))
					return
				}

				if err := publicClient.subscribeSymbols(publicClient.openInventorySymbols(wallet)); err != nil {
					fail(err)
					return
				}
			}
		}
	})

	done := make(chan struct{})

	go func() {
		workers.Wait()
		close(done)
	}()

	select {
	case err := <-errs:
		workers.Wait()
		return errnie.Error(err)
	case <-publicClient.ctx.Done():
		workers.Wait()
		return publicClient.ctx.Err()
	case <-done:
		return publicClient.ctx.Err()
	}
}

func (publicClient *PublicClient) openInventorySymbols(tradingWallet *wallet.Wallet) []string {
	// InventoryCopy snapshots the wallet under its own mutex; iterating
	// the live Inventory map without that lock would panic with
	// "concurrent map iteration and map write" the moment a fill arrives.
	inventory := tradingWallet.InventoryCopy()
	symbols := make([]string, 0, len(inventory))

	for base, qty := range inventory {
		if qty <= config.System.LiveInventoryEpsilon {
			continue
		}

		symbols = append(symbols, base+"/"+tradingWallet.Currency)
	}

	return symbols
}

func (publicClient *PublicClient) subscribeSymbols(symbols []string) error {
	publicClient.mu.Lock()
	defer publicClient.mu.Unlock()

	if publicClient.replay {
		return nil
	}

	pending := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		publicClient.heldSymbols[symbol] = struct{}{}

		if _, seen := publicClient.subscribed[symbol]; seen {
			continue
		}

		publicClient.subscribed[symbol] = struct{}{}
		pending = append(pending, symbol)
	}

	if len(pending) == 0 || publicClient.conn == nil {
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
}

func (publicClient *PublicClient) flushHeldSymbols() error {
	publicClient.mu.Lock()

	if len(publicClient.heldSymbols) == 0 {
		publicClient.mu.Unlock()
		return nil
	}

	resubscribe := make([]string, 0, len(publicClient.heldSymbols))

	for symbol := range publicClient.heldSymbols {
		resubscribe = append(resubscribe, symbol)
	}

	publicClient.mu.Unlock()

	return publicClient.subscribeSymbols(resubscribe)
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
	for publicClient.ctx.Err() == nil {
		if err := publicClient.replayOnce(); err != nil {
			errnie.Error(err)
			return
		}

		if !config.System.ReplayLoop {
			return
		}
	}
}

func (publicClient *PublicClient) replayOnce() error {
	file, err := os.Open(config.System.ReplayFile)

	if err != nil {
		return err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	for publicClient.ctx.Err() == nil && scanner.Scan() {
		line := scanner.Bytes()

		if len(line) == 0 {
			continue
		}

		publicClient.read(line)
	}

	return scanner.Err()
}

/*
IngestReplayLine routes one captured WebSocket payload through the public
client demux path. Replay fixtures and profile harnesses use this entry point.
*/
func (publicClient *PublicClient) IngestReplayLine(line []byte) {
	publicClient.read(line)
}

func (publicClient *PublicClient) runLive() {
	errnie.Info("starting public client live")

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

		publicClient.mu.Lock()
		publicClient.conn = conn
		publicClient.subscribed = make(map[string]struct{})
		publicClient.catalogAt = time.Time{}
		publicClient.mu.Unlock()

		if err := publicClient.flushHeldSymbols(); err != nil {
			errnie.Error(err)
			conn.Close()
			publicClient.mu.Lock()
			publicClient.conn = nil
			publicClient.mu.Unlock()

			select {
			case <-publicClient.ctx.Done():
				return
			case <-time.After(backoff):
			}

			continue
		}

		if err := conn.WriteJSON(instrument.NewSubscribe()); err != nil {
			errnie.Error(err)
			conn.Close()
			publicClient.mu.Lock()
			publicClient.conn = nil
			publicClient.mu.Unlock()

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
					publicClient.mu.Lock()
					if err := conn.WriteJSON(map[string]string{"method": "ping"}); err != nil {
						publicClient.mu.Unlock()
						return
					}
					publicClient.mu.Unlock()
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
		publicClient.mu.Lock()
		publicClient.conn = nil
		publicClient.mu.Unlock()

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

	switch {
	case channel == core.ChannelInstrument:
		var message struct {
			Type string `json:"type"`
			Data struct {
				Pairs []instrument.Data `json:"pairs"`
			} `json:"data"`
		}

		if json.Unmarshal(payload, &message) != nil {
			return
		}

		publicClient.mu.Lock()
		catalogAt := publicClient.catalogAt
		publicClient.mu.Unlock()

		if message.Type != "snapshot" && !catalogAt.IsZero() {
			return
		}

		if !catalogAt.IsZero() {
			refresh := 6 * time.Hour

			if time.Since(catalogAt) < refresh {
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

		publicClient.mu.Lock()
		publicClient.catalogAt = time.Now()
		publicClient.subscribed = make(map[string]struct{})
		publicClient.mu.Unlock()

		publicClient.symbols.Send(&qpool.QValue[any]{Value: pairs})

		_ = publicClient.flushHeldSymbols()

		publicClient.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event": "quote_progress",
			"ts":    time.Now().UTC().Format(time.RFC3339Nano),
			"ready": 0,
			"total": len(pairs),
		}})
	case channel == core.ChannelTicker:
		rows, err := market.ParseTickerRows(payload)

		if err != nil {
			return
		}

		for _, row := range rows {
			publicClient.tick.Send(&qpool.QValue[any]{Value: row})

			price := row.Last

			if price <= 0 && row.Bid > 0 && row.Ask > 0 {
				price = (row.Bid + row.Ask) / 2
			}

			if price <= 0 {
				continue
			}

			publicClient.ui.Send(&qpool.QValue[any]{Value: map[string]any{
				"event":  "mark",
				"ts":     tickerTimestamp(row, time.Now().UTC()),
				"symbol": row.Symbol,
				"price":  price,
			}})
		}
	case market.Channel(channel).IsTrade():
		var message trade.Snapshot

		if json.Unmarshal(payload, &message) != nil {
			return
		}

		for _, row := range message.Data {
			publicClient.trade.Send(&qpool.QValue[any]{Value: row})

			if row.Price <= 0 {
				continue
			}

			publicClient.ui.Send(&qpool.QValue[any]{Value: map[string]any{
				"event":  "mark",
				"ts":     row.Timestamp.UTC().Format(time.RFC3339Nano),
				"symbol": row.Symbol,
				"price":  row.Price,
			}})
		}
	case channel == core.ChannelOHLC:
		var message ohlc.Snapshot

		if json.Unmarshal(payload, &message) != nil {
			return
		}

		now := time.Now().UTC()

		for _, row := range message.Data {
			publicClient.ohlc.Send(&qpool.QValue[any]{Value: row})
			publicClient.ui.Send(&qpool.QValue[any]{Value: map[string]any{
				"event":  "candle_bar",
				"ts":     timestampOrNow(row.IntervalBegin, now),
				"symbol": row.Symbol,
				"sec":    unixTimestampOrNow(row.IntervalBegin, now),
				"open":   row.Open,
				"high":   row.High,
				"low":    row.Low,
				"close":  row.Close,
				"volume": row.Volume,
			}})

			if row.Close <= 0 {
				continue
			}

			publicClient.ui.Send(&qpool.QValue[any]{Value: map[string]any{
				"event":  "mark",
				"ts":     timestampOrNow(row.IntervalBegin, now),
				"symbol": row.Symbol,
				"price":  row.Close,
			}})
		}
	case market.Channel(channel).IsBook():
		delta, err := market.ParseBookLevelsDeltaWithDepth(payload, config.System.BookDepthLevels)

		if err != nil {
			return
		}

		publicClient.book.Send(&qpool.QValue[any]{Value: delta})
	}
}

func tickerTimestamp(row market.TickerRow, now time.Time) string {
	timestamp, err := time.Parse(time.RFC3339Nano, row.Timestamp)

	if err != nil || timestamp.IsZero() {
		return now.UTC().Format(time.RFC3339Nano)
	}

	return timestamp.UTC().Format(time.RFC3339Nano)
}

func timestampOrNow(timestamp time.Time, now time.Time) string {
	if timestamp.IsZero() {
		return now.UTC().Format(time.RFC3339Nano)
	}

	return timestamp.UTC().Format(time.RFC3339Nano)
}

func unixTimestampOrNow(timestamp time.Time, now time.Time) int64 {
	if timestamp.IsZero() {
		return now.UTC().Unix()
	}

	return timestamp.UTC().Unix()
}
