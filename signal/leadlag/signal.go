package leadlag

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/symm/kraken/websocket"
	signalshared "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal is the Anchor perspective, measuring temporal correlation between the
cross-section leader and each follower. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	section       *Section
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	mu            sync.Mutex
}

/*
NewSignal creates lead-lag measurement state for central market cuts so
temporal relationships persist across Thesis ticks.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	planner *strategy.Planner,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		api:           api,
		section:       NewSection(),
		planner:       planner,
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
	}
	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceLeadLag)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return signalshared.Subscribe(
		&signal.mu,
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) run() {
	subscription := signal.subscriptions["thesis"]

	if subscription == nil {
		return
	}

	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-subscription.Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					thesis.AppendMeasurements(
						types.SourceLeadLag,
						signal.Measure(thesis),
						types.Stamp{At: time.Now(), Entity: types.MarketTicker},
					)

					utils.Fanout(signal.subscribers, signal.Name(), thesis)
				}

			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	if signal.section == nil {
		signal.section = NewSection()
	}

	tickers, _, _ := thesis.Market()

	if thesis.CrossSection == nil {
		return nil
	}

	crossSection := thesis.CrossSection

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
