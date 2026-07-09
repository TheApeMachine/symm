package trader

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/ohlc"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

const (
	channelInstrument = "instrument"
	channelTicker     = "ticker"
	channelTrade      = "trade"
	channelOHLC       = "ohlc"
	channelBook       = "book"
	channelLevel3     = "level3"
)

/*
Crypto is the simple trading runtime.
It consumes market and private frames, publishes UI frames,
and delegates measurement to Signal.
*/
type Crypto struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      chan error
	tree     *dmt.Tree
	channels map[string]chan []byte
	uiHub    *ui.Hub
	desk     *broker.Desk
	private  websocket.Private
	status   atomic.Value
	ticker   *Ticker
	trade    *Trade
	ohlc     *OHLC
	book     *Book
	level3   *Level3
	tick     *atomic.Int64
	quote    string
	schedule *sync.Map
	spreads  *sync.Map
	planner  *Planner
	analyzer *logic.Analyzer
}

/*
NewCrypto wires the trading runtime around shared infrastructure.
*/
func NewCrypto(
	ctx context.Context,
	tree *dmt.Tree,
	private websocket.Private,
	public websocket.PublicSocket,
	uiHub *ui.Hub,
	level3Sockets ...websocket.Socket,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	desk, err := broker.NewDesk(
		ctx, public, private, uiHub.Messages,
	)

	if err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			err.Error(),
			err,
		))
	}

	crossSection, err := types.NewCrossSection(
		types.DefaultCrossSectionConfig(),
	)

	if err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			err.Error(),
			err,
		))
	}

	channels := map[string]chan []byte{
		channelInstrument: public.Observe(channelInstrument),
		channelTicker:     public.Observe(channelTicker),
		channelTrade:      public.Observe(channelTrade),
		channelOHLC:       public.Observe(channelOHLC),
		channelBook:       public.Observe(channelBook),
	}

	for _, level3Socket := range level3Sockets {
		channels[channelLevel3] = level3Socket.Observe(channelLevel3)
	}

	price := broker.NewPrice(ctx, public, private)

	if price == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"trader: broker price required",
			nil,
		))
	}

	correlationSignal := correlation.NewSignal[any](ctx)
	cvdSignal := cvd.NewSignal[any](ctx)
	depthflowSignal := depthflow.NewSignal[any](ctx)
	exhaustSignal := exhaust.NewSignal[any](ctx)
	fluidSignal := fluid.NewSignal[any](ctx)
	hawkesSignal := hawkes.NewSignal[any](ctx)
	leadlagSignal := leadlag.NewSignal[any](ctx)
	liquiditySignal := liquidity.NewSignal[any](ctx)
	pumpdumpSignal := pumpdump.NewSignal[any](ctx)
	sentimentSignal := sentiment.NewSignal[any](ctx)
	toxicitySignal := toxicity.NewSignal[any](ctx)
	ohlcSignal := ohlc.NewSignal[any](ctx)

	crypto := &Crypto{
		ctx:      ctx,
		cancel:   cancel,
		tree:     tree,
		channels: channels,
		desk:     desk,
		private:  private,
		uiHub:    uiHub,
		ticker: NewTicker([]types.Signal[any]{
			correlationSignal,
			fluidSignal,
			leadlagSignal,
			liquiditySignal,
			pumpdumpSignal,
			sentimentSignal,
		}, crossSection),
		trade: NewTrade([]types.Signal[any]{
			cvdSignal,
			depthflowSignal,
			exhaustSignal,
			fluidSignal,
			hawkesSignal,
			pumpdumpSignal,
			toxicitySignal,
		}),
		ohlc: NewOHLC([]types.Signal[any]{
			ohlcSignal,
		}),
		book: NewBook([]types.Signal[any]{
			depthflowSignal,
			exhaustSignal,
			fluidSignal,
			pumpdumpSignal,
		}),
		level3: NewLevel3([]types.Signal[any]{
			toxicitySignal,
		}),
		tick:     &atomic.Int64{},
		quote:    viper.GetViper().GetString("market.quote_currency"),
		schedule: &sync.Map{},
		spreads:  &sync.Map{},
		planner:  NewPlanner(desk, price),
		analyzer: logic.NewAnalyzer(nil, tree, uiHub),
	}

	// Market data (book/trade/ohlc/level3) cannot be measured until the
	// instrument snapshot has been ingested — book.annotate needs the price
	// increment per symbol. Start INITIALIZING and flip to READY on the first
	// instrument frame; gate the market-data cases on it in Run.
	crypto.status.Store(types.INITIALIZING)

	return crypto, nil
}

/*
ready reports whether the instrument snapshot has been ingested and the fee
schedule installed. Market data (book/trade/ohlc/level3) measurement depends on
per-symbol instrument metadata (e.g. the book price increment) and real fees, so
it must not run while INITIALIZING.
*/
func (crypto *Crypto) ready() bool {
	status := crypto.status.Load().(types.Status)
	//errnie.Debug("crypto: status - " + string(status))
	return status == types.READY
}

/*
Run processes websocket and private frame streams until ctx closes.
*/
func (crypto *Crypto) Run() error {
	go func() {
		if runErr := crypto.desk.Run(); runErr != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"trader: desk execution failed",
				runErr,
			))
		}
	}()

	measurements := make([]*types.Measurement, 0)

	// Instrument Ingestion Worker
	go func() {
		for {
			select {
			case <-crypto.ctx.Done():
				return
			case msg, ok := <-crypto.channels[channelInstrument]:
				if !ok {
					return
				}

				instrumentData := kraken.NewInstrumentData(msg)
				crypto.book.ObserveInstruments(instrumentData)

				pairs := make([]string, 0, len(instrumentData.Pairs))

				for _, pair := range instrumentData.Pairs {

					if pair.Symbol == "" || pair.Status != "online" || pair.Quote != crypto.quote {
						continue
					}

					pairs = append(pairs, pair.Symbol)
				}

				feesReady := crypto.ready()

				if !feesReady && len(pairs) > 0 {
					schedule, err := crypto.private.TradeVolume(pairs)

					if err != nil {
						errnie.Error(errnie.Err(
							errnie.Internal,
							err.Error(),
							err,
						))
					}

					for key, value := range schedule.Pairs {
						crypto.schedule.Store(key, value)
					}

					if err == nil && schedule.Fallback.Taker > 0 {
						errnie.Error(crypto.desk.SetFeeSchedule(schedule))
						feesReady = true
					}
				}

				if !crypto.ready() && feesReady {
					errnie.Info("crypto: system is READY, activating market data measurements")
					crypto.status.Store(types.READY)
				}

				var instruments any

				if err := sonic.Unmarshal(msg, &instruments); err != nil {
					errnie.Error(errnie.Err(
						errnie.UnprocessableContent,
						err.Error(),
						err,
					))
				}

				crypto.uiHub.Messages <- datura.Map[any]{
					"instruments": instruments,
				}.Marshal()
			case msg, ok := <-crypto.channels[channelTicker]:
				if !ok {
					return
				}

				if !crypto.ready() {
					continue
				}

				tickers := kraken.NewTickerDataSlice(msg)

				for _, ticker := range tickers {
					askRat := ticker.Ask.Rat()
					bidRat := ticker.Bid.Rat()
					two := big.NewRat(2, 1)
					midRat := new(big.Rat).Quo(new(big.Rat).Add(askRat, bidRat), two)

					if midRat.Sign() > 0 {
						spreadRat := new(big.Rat).Quo(new(big.Rat).Sub(askRat, bidRat), midRat)
						spreadDec, err := decimal.NewFromString(spreadRat.FloatString(8))

						if err == nil {
							crypto.spreads.Store(ticker.Symbol, *spreadDec)
						}
					}
				}

				measurements = errnie.Does(func() ([]*types.Measurement, error) {
					return crypto.ticker.Measure(tickers)
				}).Value()
			case msg, ok := <-crypto.channels[channelTrade]:
				if !ok {
					return
				}

				if !crypto.ready() {
					continue
				}

				trades := kraken.NewTradeDataSlice(msg)

				measurements = errnie.Does(func() ([]*types.Measurement, error) {
					return crypto.trade.Measure(trades)
				}).Value()
			case msg, ok := <-crypto.channels[channelOHLC]:
				if !ok {
					return
				}

				if !crypto.ready() {
					continue
				}

				ohlc := kraken.NewOHLCDataSlice(msg)

				measurements = errnie.Does(func() ([]*types.Measurement, error) {
					return crypto.ohlc.Measure(ohlc)
				}).Value()
			case msg, ok := <-crypto.channels[channelBook]:
				if !ok {
					return
				}

				if !crypto.ready() {
					continue
				}

				book := kraken.NewBookDataSlice(msg)

				measurements = errnie.Does(func() ([]*types.Measurement, error) {
					return crypto.book.Measure(book)
				}).Value()
			case msg, ok := <-crypto.channels[channelLevel3]:
				if !ok {
					return
				}

				if !crypto.ready() {
					continue
				}

				level3 := kraken.NewLevel3DataSlice(msg)

				measurements = errnie.Does(func() ([]*types.Measurement, error) {
					return crypto.level3.Measure(level3)
				}).Value()
			}

			if !crypto.ready() {
				continue
			}

			theses := crypto.analyzer.Update(measurements)
			intents, err := crypto.planner.Update(theses)

			if err != nil {
				errnie.Error(err)
			}

			tick := crypto.tick.Add(1)

			out := datura.Map[any]{
				"tick": datura.Map[any]{
					"count":        tick,
					"phase":        "stream",
					"measurements": len(measurements),
					"open":         crypto.desk.OpenPositions(),
				},
			}

			if len(measurements) > 0 {
				out["measurements"] = measurements
			}

			if len(intents) > 0 {
				out["intents"] = intents
			}

			crypto.uiHub.Messages <- out.Marshal()
		}
	}()

	return nil
}

/*
Close stops the trader and its composed signal resources.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()

	if err := crypto.desk.Close(); err != nil {
		return err
	}

	if crypto.analyzer != nil {
		crypto.analyzer.Close()
	}

	return nil
}
