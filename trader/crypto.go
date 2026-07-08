package trader

import (
	"context"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
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
	ctx       context.Context
	cancel    context.CancelFunc
	err       chan error
	tree      *dmt.Tree
	channels  map[string]chan []byte
	uiHub     *ui.Hub
	desk      *broker.Desk
	private   websocket.Private
	status    atomic.Value
	ticker    *Ticker
	trade     *Trade
	ohlc      *OHLC
	book      *Book
	level3    *Level3
	decision  *logic.Decision
	portfolio *Portfolio
	cortex    *Cortex
	tick      *atomic.Int64
	quote     string
	schedule  *sync.Map
	spreads   *sync.Map
}

/*
NewCrypto wires the trading runtime around shared infrastructure.
*/
func NewCrypto(
	ctx context.Context,
	tree *dmt.Tree,
	private websocket.Private,
	socket websocket.Socket,
	uiHub *ui.Hub,
	level3Sockets ...websocket.Socket,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	desk, err := broker.NewDesk(
		ctx, socket, private, uiHub.Messages,
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
		channelInstrument: socket.Observe(channelInstrument),
		channelTicker:     socket.Observe(channelTicker),
		channelTrade:      socket.Observe(channelTrade),
		channelOHLC:       socket.Observe(channelOHLC),
		channelBook:       socket.Observe(channelBook),
	}

	for _, level3Socket := range level3Sockets {
		channels[channelLevel3] = level3Socket.Observe(channelLevel3)
	}

	recorder, err := newAuditRecorder()

	if err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			err.Error(),
			err,
		))
	}

	portfolio, err := NewPortfolio(recorder)

	if err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			err.Error(),
			err,
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
		ohlc: NewOHLC([]types.Signal[any]{}),
		book: NewBook([]types.Signal[any]{
			depthflowSignal,
			exhaustSignal,
			fluidSignal,
			pumpdumpSignal,
		}),
		level3: NewLevel3([]types.Signal[any]{
			toxicitySignal,
		}),
		decision:  logic.NewDecision(recorder),
		portfolio: portfolio,
		cortex:    newCortex(tree),
		tick:      &atomic.Int64{},
		quote:     viper.GetViper().GetString("market.quote_currency"),
		schedule:  &sync.Map{},
		spreads:   &sync.Map{},
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
type marketEvent struct {
	kind   string
	ticker kraken.TickerDataSlice
	trade  kraken.TradeDataSlice
	ohlc   kraken.OHLCDataSlice
	book   kraken.BookDataSlice
	level3 kraken.Level3DataSlice
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

	measurementRing, err := structure.NewMPMCRing[*marketEvent](crypto.ctx, 4096)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			err.Error(),
			err,
		))
	}

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
			}
		}
	}()

	// Ticker Ingestion Worker
	go func() {
		for {
			select {
			case <-crypto.ctx.Done():
				return
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

				measurementRing.Push(&marketEvent{
					kind:   channelTicker,
					ticker: tickers,
				})
			}
		}
	}()

	// Trade Ingestion Worker
	go func() {
		for {
			select {
			case <-crypto.ctx.Done():
				return
			case msg, ok := <-crypto.channels[channelTrade]:

				if !ok {
					return
				}

				if !crypto.ready() {
					continue
				}

				trades := kraken.NewTradeDataSlice(msg)

				measurementRing.Push(&marketEvent{
					kind:  channelTrade,
					trade: trades,
				})
			}
		}
	}()

	// OHLC Ingestion Worker
	go func() {
		for {
			select {
			case <-crypto.ctx.Done():
				return
			case msg, ok := <-crypto.channels[channelOHLC]:

				if !ok {
					return
				}

				if !crypto.ready() {
					continue
				}

				ohlc := kraken.NewOHLCDataSlice(msg)

				measurementRing.Push(&marketEvent{
					kind: channelOHLC,
					ohlc: ohlc,
				})
			}
		}
	}()

	// Book Ingestion Worker
	go func() {
		for {
			select {
			case <-crypto.ctx.Done():
				return
			case msg, ok := <-crypto.channels[channelBook]:

				if !ok {
					return
				}

				if !crypto.ready() {
					continue
				}

				book := kraken.NewBookDataSlice(msg)

				measurementRing.Push(&marketEvent{
					kind: channelBook,
					book: book,
				})
			}
		}
	}()

	// Level3 Ingestion Worker
	go func() {
		for {
			select {
			case <-crypto.ctx.Done():
				return
			case msg, ok := <-crypto.channels[channelLevel3]:

				if !ok {
					return
				}

				if !crypto.ready() {
					continue
				}

				level3 := kraken.NewLevel3DataSlice(msg)

				measurementRing.Push(&marketEvent{
					kind:   channelLevel3,
					level3: level3,
				})
			}
		}
	}()

	// Main Execution loop
	go func() {
		for {

			if crypto.ctx.Err() != nil {
				crypto.Close()
				return
			}

			event := measurementRing.Pop()

			if event == nil {
				// Spin backoff
				for index := 0; index < 50; index++ {
					event = measurementRing.Pop()

					if event != nil {
						break
					}

					runtime.Gosched()
				}

				if event == nil {
					// Fall back to a short sleep to yield CPU
					time.Sleep(time.Millisecond)
					continue
				}
			}

			var measurements []*types.Measurement
			var measureErr error

			switch event.kind {
			case channelTicker:
				measurements, measureErr = crypto.ticker.Measure(event.ticker)
			case channelTrade:
				measurements, measureErr = crypto.trade.Measure(event.trade)
			case channelOHLC:
				measurements, measureErr = crypto.ohlc.Measure(event.ohlc)
			case channelBook:
				measurements, measureErr = crypto.book.Measure(event.book)
			case channelLevel3:
				measurements, measureErr = crypto.level3.Measure(event.level3)
			}

			if measureErr != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent, measureErr.Error(), measureErr,
				))
				continue
			}

			if !crypto.ready() {
				continue
			}

			decision := errnie.Does(func() (logic.Batch, error) {
				return crypto.decision.Measure(measurements)
			}).Or(func(err error) {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}).Value()

			cognitive, err := crypto.cortex.Measure(measurements, decision)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}

			if err == nil {
				for symbol, reading := range cognitive {
					priorMass := reading.PriorMass()

					if err := crypto.decision.SetPrior(symbol, logic.DecisionPrior{
						TopdownPhaseScale:  priorMass,
						TopdownEnergyScale: priorMass,
					}); err != nil {
						errnie.Error(errnie.Err(
							errnie.UnprocessableContent,
							err.Error(),
							err,
						))
					}
				}
			}

			tradingReady := crypto.execute(
				decision.Actions,
				decision.Momentum(),
				decision.Continuation(),
			)

			tick := crypto.tick.Add(1)

			out := datura.Map[any]{
				"tick": datura.Map[any]{
					"count":        tick,
					"phase":        "stream",
					"measurements": len(measurements),
					"candidates":   len(decision.Actions),
					"open":         crypto.desk.OpenPositions(),
					"ready":        tradingReady,
				},
			}

			if len(measurements) > 0 {
				out["measurements"] = measurements
			}

			if len(decision.Manifold) > 0 {
				out["manifold"] = decision.Manifold
			}

			if len(decision.Resonance) > 0 {
				out["resonance"] = decision.Resonance
			}

			if len(decision.Causal) > 0 {
				out["causal"] = decision.Causal
			}

			if len(cognitive) > 0 {
				out["cognitive"] = datura.Map[any]{
					"readings": cognitive,
				}
			}

			if len(decision.Actions) > 0 {
				out["actions"] = decision.Actions
			}

			if stops := crypto.portfolio.Stops(); len(stops) > 0 {
				out["stops"] = stops
			}

			go func(outputMap datura.Map[any]) {
				crypto.uiHub.Messages <- outputMap.Marshal()
			}(out)
		}
	}()

	return nil
}

func (crypto *Crypto) execute(
	actions []*logic.Action,
	momentum map[string]float64,
	continuation map[string]float64,
) bool {
	// Reconcile always runs so exits are evaluated every tick regardless of
	// desk state — a close is never gated. The desk is the single authority on
	// capacity: Sell always executes; Buy accepts or rejects against the
	// READY/PRIORITY/BUSY state. Entries are additionally held until the account
	// has hydrated, since a Buy cannot be sized without a balance snapshot.
	tradingReady := crypto.desk.Ready()

	holdings := crypto.desk.Holdings()

	for symbol, holding := range holdings {
		if val, ok := crypto.spreads.Load(symbol); ok {
			holding.Spread = val.(decimal.Decimal)
		}

		if pair, ok := crypto.book.Instrument(symbol); ok {
			holding.PriceIncrement = pair.Increment()
		}

		holdings[symbol] = holding
	}

	for _, intent := range crypto.portfolio.Reconcile(
		actions, holdings, momentum, continuation,
	) {
		if intent.kind == intentExit {
			if err := crypto.desk.Sell(intent.symbol); err != nil {
				errnie.Error(err)
				crypto.portfolio.Abort(intent.symbol)
			}

			continue
		}

		if !tradingReady {
			// Not hydrated yet: drop the entry so a thesis is not stranded on a
			// buy that never executes.
			crypto.portfolio.Abort(intent.symbol)
			continue
		}

		if err := crypto.desk.Buy(
			intent.symbol,
			intent.fraction,
			intent.price,
			false,
		); err != nil {
			errnie.Error(err)
			crypto.portfolio.Abort(intent.symbol)
		}
	}

	return tradingReady
}

/*
Close stops the trader and its composed signal resources.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()

	crypto.decision.Close()

	if err := crypto.desk.Close(); err != nil {
		return err
	}

	return nil
}

/*
newAuditRecorder builds the diagnostic recorder when system.audit is enabled,
returning nil when it is not so the decision ladder simply records nothing.
*/
func newAuditRecorder() (*audit.Recorder, error) {
	if !viper.GetBool("system.audit.enabled") {
		return nil, nil
	}

	file := viper.GetString("system.audit.file")

	if file == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"trader: system.audit.file required when audit is enabled",
			nil,
		))
	}

	return audit.NewRecorder(file)
}
