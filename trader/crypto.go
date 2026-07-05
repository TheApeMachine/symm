package trader

import (
	"context"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
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
	ctx      context.Context
	cancel   context.CancelFunc
	tree     *dmt.Tree
	channels map[string]chan []byte
	ui       *UI
	desk     *broker.Desk
	signals  *Signals
	ticker   *Ticker
	trade    *Trade
	ohlc     *OHLC
	book     *Book
	level3   *Level3
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

	crypto := &Crypto{
		ctx:      ctx,
		cancel:   cancel,
		tree:     tree,
		channels: channels,
		ui:       NewUI(publisher),
		desk:     desk,
		signals:  signals,
		ticker:   NewTicker(),
		trade:    NewTrade(),
		ohlc:     NewOHLC(),
		book:     NewBook(),
		level3:   NewLevel3(),
	}

	return crypto, nil
}

/*
Run processes any supplied websocket/account frame streams until ctx closes.
*/
func (crypto *Crypto) Run() (err error) {
	var measurement types.Measurement

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case msg := <-crypto.channels[channelTicker]:
			measurement, err := crypto.ticker.Measure(kraken.NewTickerDataSlice(msg))
		case msg := <-crypto.channels[channelTrade]:
			measurement, err := crypto.trade.Measure(kraken.NewTradeDataSlice(msg))
		case msg := <-crypto.channels[channelOHLC]:
			measurement, err := crypto.ohlc.Measure(kraken.NewOHLCDataSlice(msg))
		case msg := <-crypto.channels[channelBook]:
			measurement, err := crypto.book.Measure(books)
		case msg := <-crypto.channels[channelLevel3]:
			measurement, err := crypto.level3.Measure(kraken.NewLevel3DataSlice(msg))
		}

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			))
		}
	}
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

	return nil
}
