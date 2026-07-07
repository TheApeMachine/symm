package trader

import (
	"context"
	"sync/atomic"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
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
	tree      *dmt.Tree
	channels  map[string]chan []byte
	uiHub     *ui.Hub
	desk      *broker.Desk
	ticker    *Ticker
	trade     *Trade
	ohlc      *OHLC
	book      *Book
	level3    *Level3
	decision  *logic.Decision
	portfolio *Portfolio
	cortex    *Cortex
	tick      *atomic.Int64
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
	}

	return crypto, nil
}

/*
Run processes websocket and private frame streams until ctx closes.
*/
func (crypto *Crypto) Run() (err error) {
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

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case msg := <-crypto.channels[channelInstrument]:
			crypto.book.ObserveInstruments(kraken.NewInstrumentData(msg))

			var instruments any
			if err := sonic.Unmarshal(msg, &instruments); err != nil {
				return errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				))
			}

			crypto.uiHub.Messages <- datura.Map[any]{
				"instruments": instruments,
			}.Marshal()
			continue
		case msg := <-crypto.channels[channelTicker]:
			measurements, err = crypto.ticker.Measure(kraken.NewTickerDataSlice(msg))
		case msg := <-crypto.channels[channelTrade]:
			measurements, err = crypto.trade.Measure(kraken.NewTradeDataSlice(msg))
		case msg := <-crypto.channels[channelOHLC]:
			measurements, err = crypto.ohlc.Measure(kraken.NewOHLCDataSlice(msg))
		case msg := <-crypto.channels[channelBook]:
			measurements, err = crypto.book.Measure(kraken.NewBookDataSlice(msg))
		case msg := <-crypto.channels[channelLevel3]:
			measurements, err = crypto.level3.Measure(kraken.NewLevel3DataSlice(msg))
		}

		decision, err := crypto.decision.Measure(measurements)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			))

			continue
		}

		cognitive, err := crypto.cortex.Measure(measurements, decision)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			))

			continue
		}

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

				continue
			}
		}

		tradingReady := crypto.execute(decision.Actions)

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

		crypto.uiHub.Messages <- out.Marshal()
	}
}

func (crypto *Crypto) execute(actions []*logic.Action) bool {
	if !crypto.desk.Ready() {
		return false
	}

	holdings := crypto.desk.Holdings()

	for _, intent := range crypto.portfolio.Reconcile(actions, holdings) {
		if intent.kind == intentExit {
			if err := crypto.desk.Sell(intent.symbol); err != nil {
				errnie.Error(err)
				crypto.portfolio.Abort(intent.symbol)
			}

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

	return true
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
