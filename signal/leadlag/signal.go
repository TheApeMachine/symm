package leadlag

import (
	"context"
	"sync"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Anchor perspective, measuring temporal correlation between the
cross-section leader and each follower. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	thesis  *types.Thesis
	ctx     context.Context
	cancel  context.CancelFunc
	section *Section
	ui      chan []byte
	ticker  *types.Subscription[*kraken.Ticker]
	subMu   sync.Mutex
	theses  []*types.Subscription[*types.Thesis]
}

/*
NewSignal creates lead-lag measurement state for central market cuts so
temporal relationships persist across Thesis ticks.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:     ctx,
		cancel:  cancel,
		section: NewSection(),
		ui:      ui,
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLeadLag)
}

/*
Initialize wires ticker ingress from Live. Lead-lag is ticker-cross-section
only; book and trade floods must not fill unused buffers.
*/
func (signal *Signal) Initialize(market types.MarketFeed, thesis *types.Thesis) {
	signal.thesis = thesis

	if market != nil {
		signal.ticker = market.Ticker()
	}

	go signal.run()
}

func (signal *Signal) Thesis() *types.Subscription[*types.Thesis] {
	subscription := types.NewSubscription[*types.Thesis]()
	signal.subMu.Lock()
	signal.theses = append(signal.theses, subscription)
	signal.subMu.Unlock()
	return subscription
}

func (signal *Signal) run() {
	if signal.ticker == nil {
		return
	}

	for {
		select {
		case <-signal.ctx.Done():
			return
		case ticker := <-signal.ticker.Channel:
			signal.onTicker(ticker)
		}
	}
}

func (signal *Signal) onTicker(ticker *kraken.Ticker) {
	signal.publish(signal.thesis.AppendMeasuremnts(
		types.SourceLeadLag, signal.Calculate(ticker.Data, nil, nil),
	))
}

func (signal *Signal) publish(thesis *types.Thesis) {
	if thesis == nil {
		return
	}

	signal.subMu.Lock()
	subscribers := append([]*types.Subscription[*types.Thesis](nil), signal.theses...)
	signal.subMu.Unlock()

	for _, subscription := range subscribers {
		subscription.Send(thesis)
	}
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) []*types.Measurement {
	crossSection := types.NewCrossSection()

	if len(tickers) > 0 {
		crossSection.Measure(tickers)
	}

	return signal.measureFrame(tickers, crossSection)
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
