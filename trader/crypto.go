package trader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/market"
)

const (
	channelTicker = "ticker"
	channelTrade  = "trade"
	channelOHLC   = "ohlc"
	channelBook   = "book"
	channelLevel3 = "level3"
)

/*
Crypto is the simple trading runtime.
It consumes market and account frames, stores raw artifacts, publishes UI frames,
and delegates market measurement to Signal.
*/
type Crypto struct {
	ctx       context.Context
	cancel    context.CancelFunc
	pool      *qpool.Q[any]
	tree      *dmt.Tree
	cognitive *market.CognitiveEvaluator
	balances  atomic.Pointer[datura.Artifact]
	ticks     atomic.Int64
	channels  map[string]chan []byte
	feeds     map[string]*qpool.BroadcastGroup
	ui        *qpool.BroadcastGroup
	desk      *broker.Desk
	decision  *Decision
	signals   *Signals
	story     *market.Story
	ticker    *Ticker
	trade     *Trade
	ohlc      *OHLC
	book      *Book
	level3    *Level3
}

/*
NewCrypto wires the trading runtime around shared infrastructure.
*/
func NewCrypto(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
	socket websocket.Socket,
	level3Sockets ...websocket.Socket,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	channels := map[string]chan []byte{
		channelTicker: socket.Observe(channelTicker),
		channelTrade:  socket.Observe(channelTrade),
		channelOHLC:   socket.Observe(channelOHLC),
		channelBook:   socket.Observe(channelBook),
	}

	for _, level3Socket := range level3Sockets {
		channels[channelLevel3] = level3Socket.Observe(channelLevel3)
	}

	signals, err := NewSignals(ctx, pool)
	if err != nil {
		cancel()
		return nil, err
	}

	decision, err := NewDecision()
	if err != nil {
		cancel()
		return nil, err
	}

	crypto := &Crypto{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		tree:      tree,
		cognitive: market.NewCognitiveEvaluator(tree),
		channels:  channels,
		desk:      broker.NewDesk(ctx, pool, tree),
		decision:  decision,
		signals:   signals,
		story:     market.NewStory(ctx, pool),
		ticker:    NewTicker(),
		trade:     NewTrade(),
		ohlc:      NewOHLC(),
		book:      NewBook(),
		level3:    NewLevel3(),
	}

	return crypto, nil
}

/*
Run processes any supplied websocket/account frame streams until ctx closes.
*/
func (crypto *Crypto) Run() error {
	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case msg, ok := <-crypto.channels[channelTicker]:
			if !ok {
				return errnie.Err(errnie.IO, "trader: ticker channel closed", nil)
			}

			at, err := crypto.ticker.Measure(kraken.NewTickerDataSlice(msg))

			if err != nil {
				return err
			}

			if err := crypto.measure(channelTicker, msg, at); err != nil {
				return err
			}
		case msg, ok := <-crypto.channels[channelTrade]:
			if !ok {
				return errnie.Err(errnie.IO, "trader: trade channel closed", nil)
			}

			at, err := crypto.trade.Measure(kraken.NewTradeDataSlice(msg))

			if err != nil {
				return err
			}

			if err := crypto.measure(channelTrade, msg, at); err != nil {
				return err
			}
		case msg, ok := <-crypto.channels[channelOHLC]:
			if !ok {
				return errnie.Err(errnie.IO, "trader: ohlc channel closed", nil)
			}

			at, err := crypto.ohlc.Measure(kraken.NewOHLCDataSlice(msg))

			if err != nil {
				return err
			}

			if err := crypto.measure(channelOHLC, msg, at); err != nil {
				return err
			}
		case msg, ok := <-crypto.channels[channelBook]:
			if !ok {
				return errnie.Err(errnie.IO, "trader: book channel closed", nil)
			}

			at, err := crypto.book.Measure(kraken.NewBookDataSlice(msg))

			if err != nil {
				return err
			}

			if err := crypto.measure(channelBook, msg, at); err != nil {
				return err
			}
		case msg, ok := <-crypto.channels[channelLevel3]:
			if !ok {
				return errnie.Err(errnie.IO, "trader: level3 channel closed", nil)
			}

			at, err := crypto.level3.Measure(kraken.NewLevel3DataSlice(msg))

			if err != nil {
				return err
			}

			if err := crypto.measure(channelLevel3, msg, at); err != nil {
				return err
			}
		}
	}
}

func (crypto *Crypto) measure(role string, msg []byte, at time.Time) error {
	measurements, err := crypto.signals.Measure(role, msg, at)
	if err != nil {
		return err
	}

	crypto.story.Update(measurements)

	actions := crypto.story.Actions(crypto.balances.Load())
	decisions, err := crypto.decision.Choose(actions)
	if err != nil {
		return err
	}

	return crypto.desk.Update(decisions)
}

/*
Close stops the trader and its composed signal resources.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()

	if err := crypto.signals.Close(); err != nil {
		return err
	}

	if err := crypto.desk.Close(); err != nil {
		return err
	}

	return crypto.story.Close()
}
