package leadlag

import (
	"context"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Anchor perspective, measuring temporal correlation between the
cross-section leader and each follower. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	*types.Actor
	thesis  *types.Thesis
	ctx     context.Context
	cancel  context.CancelFunc
	section *Section
	ui      chan []byte
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

	signal.Actor = types.NewActor(ctx, "leadlag", map[string]types.Handler{
		"ticker": {Topic: "thesis", Fn: signal.onTicker},
	})

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
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
	)
}

func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return types.SignalResult{Source: types.SourceLeadLag, Status: types.SignalSkip}
	}

	if len(measurements) > 0 {
		signal.thesis.Publish(types.SourceLeadLag, measurements)
		return types.SignalResult{Source: types.SourceLeadLag, Measurements: measurements, Status: types.SignalReady}
	}

	return types.SignalResult{Source: types.SourceLeadLag, Status: types.SignalSkip}
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
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
