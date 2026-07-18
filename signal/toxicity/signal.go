package toxicity

import (
	"context"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Toxicity tracks whether near-touch liquidity is sincere, retreating, or bluffing
from level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	level3     *Level3
	priorTouch map[string]touchSnapshot
	ui         chan []byte
}

func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:        ctx,
		cancel:     cancel,
		level3:     NewLevel3(api),
		priorTouch: map[string]touchSnapshot{},
		ui:         ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.WireMeasurements(measurements),
	}.Marshal():
	default:
	}
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements, err := signal.Calculate(thesis.Market())

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return measurements
}

/*
touchSnapshot retains prior best-level quantities so toxicity can distinguish
withdrawal from execution.
*/
type touchSnapshot struct {
	bidQuantity float64
	askQuantity float64
	observedAt  time.Time
}

/*
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	incrementBySymbol := priceIncrementsBySymbol(frame)
	evidence := signal.accumulateEvidence(frame.Trades, incrementBySymbol)

	out := make([]*types.Measurement, 0, len(evidence)*8)
	nextTouch := map[string]touchSnapshot{}

	for symbol, row := range evidence {
		signal.emitSymbolMeasurements(symbol, row, &out, nextTouch)
	}

	signal.priorTouch = nextTouch

	return out, nil
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
