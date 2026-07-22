package toxicity

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal tracks whether near-touch liquidity is sincere, retreating, or bluffing
from level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	level3       *Level3
	priorTouch   map[string]touchSnapshot
	touchScratch map[string]touchSnapshot
	evidence     map[string]*symbolEvidence
	increments   map[string]*decimal.Decimal
	ui           chan []byte
}

/*
NewSignal creates the Level3 honesty calculator against the production Kraken
API so tests can replace only its connections, never its market mechanics.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		level3:       NewLevel3(api),
		priorTouch:   map[string]touchSnapshot{},
		touchScratch: map[string]touchSnapshot{},
		evidence:     map[string]*symbolEvidence{},
		increments:   map[string]*decimal.Decimal{},
		ui:           ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.ForPublish(measurements),
	}.Marshal():
	default:
	}
}

/*
Interest requires the public trade tape; toxicity accumulates per-symbol
evidence from trades and corroborates each fill against the Level3 touch, which
it reads directly through PeekBook rather than the public book cut.
*/
func (signal *Signal) Interest() types.StreamInterest {
	return types.StreamTrade | types.StreamBook
}

/*
Measure returns typed measurements for the cut, or an error when the
cut cannot be measured honestly.
*/
func (signal *Signal) Measure(thesis *types.Thesis) ([]*types.Measurement, error) {
	return signal.Calculate(thesis.Market())
}

/*
touchSnapshot retains prior best-level quantities so toxicity can distinguish
withdrawal from execution.
*/
type touchSnapshot struct {
	bidPrice    decimal.Decimal
	askPrice    decimal.Decimal
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
	if frame == nil {
		return nil, fmt.Errorf("toxicity: market frame required")
	}

	signal.ensureScratch()

	if err := signal.ingestIncrements(frame.Books); err != nil {
		return nil, err
	}

	if err := signal.accumulateEvidence(frame.Trades); err != nil {
		return nil, err
	}

	if err := signal.observeBooks(frame.Books); err != nil {
		return nil, err
	}

	out := make([]*types.Measurement, 0, len(signal.evidence)*8)
	symbols := make([]string, 0, len(signal.evidence))
	clear(signal.touchScratch)

	for symbol := range signal.evidence {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	for _, symbol := range symbols {
		if err := signal.emitSymbolMeasurements(
			symbol,
			signal.evidence[symbol],
			&out,
			signal.touchScratch,
		); err != nil {
			return nil, err
		}
	}

	signal.priorTouch, signal.touchScratch = signal.touchScratch, signal.priorTouch
	clear(signal.touchScratch)

	return out, nil
}

/*
ensureScratch allocates reusable tick maps when tests construct Signal by hand.
*/
func (signal *Signal) ensureScratch() {
	if signal.priorTouch == nil {
		signal.priorTouch = map[string]touchSnapshot{}
	}

	if signal.touchScratch == nil {
		signal.touchScratch = map[string]touchSnapshot{}
	}

	if signal.evidence == nil {
		signal.evidence = map[string]*symbolEvidence{}
	}

	if signal.increments == nil {
		signal.increments = map[string]*decimal.Decimal{}
	}
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
