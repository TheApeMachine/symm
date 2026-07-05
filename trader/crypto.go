package trader

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/cognitive"
	"github.com/theapemachine/symm/cognitive/dmt"
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
It consumes market and account frames, publishes UI frames,
and delegates market measurement to Signal.
*/
type Crypto struct {
	ctx       context.Context
	cancel    context.CancelFunc
	tree      *dmt.Tree
	cognitive *cognitive.Evaluator
	channels  map[string]chan []byte
	ui        *UI
	desk      *broker.Desk
	decision  *Decision
	positions *market.Positions
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
	tree *dmt.Tree,
	publisher Publisher,
	account broker.Account,
	socket websocket.Socket,
	level3Sockets ...websocket.Socket,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	if publisher == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: dashboard publisher required",
			nil,
		))
	}

	desk, err := broker.NewDesk(ctx, account, publisher)
	if err != nil {
		cancel()
		return nil, err
	}

	channels := map[string]chan []byte{
		channelTicker: socket.Observe(channelTicker),
		channelTrade:  socket.Observe(channelTrade),
		channelOHLC:   socket.Observe(channelOHLC),
		channelBook:   socket.Observe(channelBook),
	}

	for _, level3Socket := range level3Sockets {
		channels[channelLevel3] = level3Socket.Observe(channelLevel3)
	}

	signals, err := NewSignals(ctx)
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
		tree:      tree,
		cognitive: cognitive.NewEvaluator(tree),
		channels:  channels,
		ui:        NewUI(publisher),
		desk:      desk,
		decision:  decision,
		positions: market.NewPositions(viper.GetString("market.quote_currency")),
		signals:   signals,
		story:     market.NewStory(ctx),
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

			if err := crypto.quote(msg, at); err != nil {
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

			books := kraken.BookDataSlice{}
			if err := books.Decode(msg); err != nil {
				return err
			}

			at, err := crypto.book.Measure(books)

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

func (crypto *Crypto) quote(msg []byte, at time.Time) error {
	rows := []map[string]any{}
	if err := sonic.Unmarshal(msg, &rows); err != nil {
		return errnie.Err(errnie.Validation, "trader: decode broker ticker", err)
	}

	frame := map[string]any{
		"channel":   "ticker",
		"data":      rows,
		"timestamp": at.UTC().Format(time.RFC3339Nano),
	}

	if err := crypto.desk.Observe(frame); err != nil {
		return err
	}

	if err := crypto.positions.Observe(frame); err != nil {
		return err
	}

	return crypto.publishPositions(at)
}

func (crypto *Crypto) Observe(frame map[string]any) error {
	if err := crypto.desk.Observe(frame); err != nil {
		return err
	}

	if err := crypto.positions.Observe(frame); err != nil {
		return err
	}

	return crypto.publishPositions(time.Now().UTC())
}

func (crypto *Crypto) publishPositions(at time.Time) error {
	readings, err := crypto.positions.Readings()
	if err != nil {
		return err
	}

	return crypto.ui.Positions(readings, crypto.positions.Quote(), at)
}

func (crypto *Crypto) measure(role string, msg []byte, at time.Time) error {
	measurements, snapshots, err := crypto.signals.Measure(role, msg, at)
	if err != nil {
		return err
	}

	readings := crypto.cognitive.Readings(
		measurements,
		viper.GetDuration("cognitive.tick_budget"),
	)
	market.ApplyCognitiveReadings(measurements, readings)

	regime := market.RegimeReading{}
	if role == channelTicker {
		regime = crypto.signals.crossSection.Regime()
	}

	if len(measurements) == 0 {
		return crypto.ui.Publish(role, at, regime, nil, nil, readings, snapshots)
	}

	if err := crypto.story.Update(measurements); err != nil {
		return err
	}

	holdings, err := crypto.desk.Holdings()
	if err != nil {
		return err
	}

	actions, err := crypto.story.Actions(holdings)
	if err != nil {
		return err
	}

	if err := crypto.ui.Publish(
		role,
		at,
		regime,
		measurements,
		actions,
		readings,
		snapshots,
	); err != nil {
		return err
	}

	decisions, err := crypto.decision.Choose(actions, crypto.story, at)
	if err != nil {
		return err
	}

	if err := crypto.ui.Decisions(actions); err != nil {
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
