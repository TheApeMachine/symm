package leadlag

import (
	"context"
	"runtime"
	"sync"

	"github.com/alitto/pond/v2"
	"github.com/theapemachine/symm/kraken/websocket"
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
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	mu            sync.Mutex
	pool          pond.Pool
	group         pond.TaskGroup
}

/*
NewSignal creates lead-lag measurement state for central market cuts so
temporal relationships persist across Thesis ticks.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
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
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
		pool:          pond.NewPool(runtime.GOMAXPROCS(0)),
	}
	signal.group = signal.pool.NewGroup()
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
	return utils.Subscribe(
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
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.AppendMeasurements(measurements, true)
						utils.Fanout(signal.subscribers, signal.Name(), thesis)
					}
				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	signal.mu.Lock()
	defer signal.mu.Unlock()

	if signal.section == nil {
		signal.section = NewSection()
	}

	tickers := thesis.MarketTickers()

	return signal.measureFrame(tickers)
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.mu.Lock()
	defer signal.mu.Unlock()

	if signal.cancel != nil {
		signal.cancel()
	}

	if signal.pool != nil {
		signal.pool.StopAndWait()
	}

	return nil
}
