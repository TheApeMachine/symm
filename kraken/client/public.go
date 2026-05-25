package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/core"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/ohlc"
	"github.com/theapemachine/symm/kraken/trade"
)

/*
PublicClient maintains an unauthenticated Kraken WebSocket v2 session.
It owns dial, ping, subscribe, and framed reads for public market channels.
*/
type PublicClient struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q
	broadcasts  map[string]*qpool.BroadcastGroup
	subscribers map[string]*qpool.Subscriber
	conn        *websocket.Conn
	url         string
	once        sync.Once
	subscribed  map[string]struct{}
}

/*
NewPublicClient creates a public websocket client bound to parent context cancellation.
*/
func NewPublicClient(
	ctx context.Context,
	pool *qpool.Q,
	url string,
) *PublicClient {
	ctx, cancel := context.WithCancel(ctx)

	client := &PublicClient{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.Subscriber),
		url:         url,
	}

	client.broadcasts["subscriptions"] = pool.CreateBroadcastGroup("subscriptions", 10*time.Millisecond)
	client.subscribers["subscriptions"] = client.broadcasts["subscriptions"].Subscribe("subscriptions", 128)

	client.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	client.subscribers["ui"] = client.broadcasts["ui"].Subscribe("ui", 128)

	if errnie.Error(errnie.Require(map[string]any{
		"ctx":         ctx,
		"cancel":      cancel,
		"pool":        pool,
		"broadcasts":  client.broadcasts,
		"subscribers": client.subscribers,
		"url":         url,
	})) != nil {
		return nil
	}

	return client
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

		if publicClient.conn == nil {
			return errnie.Error(fmt.Errorf("public client not connected"))
		}

		if err := publicClient.conn.WriteJSON(ohlc.NewSubscribe(symbols)); err != nil {
			return errnie.Error(err)
		}

		if err := publicClient.conn.WriteJSON(trade.NewSubscribe(symbols)); err != nil {
			return errnie.Error(err)
		}

		if err := publicClient.conn.WriteJSON(map[string]any{
			"method": "subscribe",
			"params": market.SubscribeParams{}.Ticker(symbols),
		}); err != nil {
			return errnie.Error(err)
		}

		if err := publicClient.conn.WriteJSON(map[string]any{
			"method": "subscribe",
			"params": market.SubscribeParams{}.Book(
				symbols, config.System.BookDepthLevels,
			),
		}); err != nil {
			return errnie.Error(err)
		}

		return nil
	default:
		return nil
	}
}

/*
Connect dials the Kraken v2 websocket endpoint.
Replay sources connect without dialing and require StartReplay after handlers register.
*/
func (publicClient *PublicClient) Connect() error {
	publicClient.ReadLoop()
	return nil
}

/*
Close cancels the session context and closes the underlying socket.
*/
func (publicClient *PublicClient) Close() error {
	publicClient.cancel()

	if publicClient.conn == nil {
		return nil
	}

	return publicClient.conn.Close()
}

func (publicClient *PublicClient) ReadLoop() {
	publicClient.once.Do(func() {
		go func() {
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

				if err := conn.WriteJSON(map[string]any{
					"method": "subscribe",
					"params": market.SubscribeParams{}.Instrument(),
				}); err != nil {
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

					publicClient.broadcasts["ui"].Send(&qpool.QValue[any]{
						Value: payload,
					})

					channel, err := market.ChannelName(payload)

					if err != nil {
						continue
					}

					switch channel {
					case core.ChannelInstrument:
						var frame market.InstrumentMessage

						if frame.Parse(payload) != nil {
							continue
						}

						pairs := make(map[string]*asset.Pair, len(frame.Data.Pairs))
						symbols := make([]string, 0, len(frame.Data.Pairs))

						for _, instrument := range frame.Data.Pairs {
							if instrument.Status != market.PairStatusOnline {
								continue
							}

							pair := instrument.AssetPair()
							pairs[instrument.Symbol] = &pair
							symbols = append(symbols, instrument.Symbol)
						}

						if len(pairs) > 0 {
							publicClient.pool.CreateBroadcastGroup("symbols", 10*time.Millisecond).Send(&qpool.QValue[any]{
								Value: pairs,
							})
						}

						pending := make([]string, 0, len(symbols))

						for _, symbol := range symbols {
							if _, seen := publicClient.subscribed[symbol]; seen {
								continue
							}

							publicClient.subscribed[symbol] = struct{}{}
							pending = append(pending, symbol)
						}

						batch := config.System.SubscribeBatch

						for start := 0; start < len(pending); start += batch {
							end := start + batch

							if end > len(pending) {
								end = len(pending)
							}

							publicClient.broadcasts["subscriptions"].Send(&qpool.QValue[any]{
								Value: pending[start:end],
							})
						}
					case core.ChannelTicker:
						rows, err := market.ParseTickerRows(payload)

						if err != nil {
							continue
						}

						tick := publicClient.pool.CreateBroadcastGroup("tick", 10*time.Millisecond)

						for _, row := range rows {
							tick.Send(&qpool.QValue[any]{Value: row})
						}
					case core.ChannelTrades, "trade":
						var frame trade.Snapshot

						if json.Unmarshal(payload, &frame) != nil {
							continue
						}

						tradeGroup := publicClient.pool.CreateBroadcastGroup("trade", 10*time.Millisecond)

						for _, data := range frame.Data {
							tradeGroup.Send(&qpool.QValue[any]{Value: data})
						}
					case core.ChannelOHLC:
						var frame ohlc.Snapshot

						if json.Unmarshal(payload, &frame) != nil {
							continue
						}

						ohlcGroup := publicClient.pool.CreateBroadcastGroup("ohlc", 10*time.Millisecond)

						for _, data := range frame.Data {
							ohlcGroup.Send(&qpool.QValue[any]{Value: data})
						}
					case core.ChannelBook:
						delta, err := market.ParseBookLevelsDelta(payload)

						if err != nil {
							continue
						}

						publicClient.pool.CreateBroadcastGroup("book", 10*time.Millisecond).Send(&qpool.QValue[any]{
							Value: delta,
						})
					}
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
		}()
	})
}
