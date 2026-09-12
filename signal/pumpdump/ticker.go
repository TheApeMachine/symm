package pumpdump

import (
	"context"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the volume-clock activity measuring instrument. It composes its
market entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close. It satisfies
nomagique/runtime.Node[*types.Envelope], dispatching on the envelope's
TypeID to whichever entity that data stream feeds.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	ticker *Ticker
	trade  *Trade
	level3 *Level3
}

/*
NewSignal composes the Ticker (executable spread), Trade (volume clock), and
Level3 (authoritative book touch) entities. quote supplies Trade with the
contemporaneous executable touch used for completed-bar midpoint response.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	trade := NewTrade(api)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
		trade:  trade,
		level3: NewLevel3(),
	}
}

func (signal *Signal) Name() string { return "pumpdump" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	switch envelope.TypeID {
	case types.EnvelopeTicker:
		envelope.PumpDump = signal.StepTicker(envelope.TickerData)
	case types.EnvelopeTrade:
		envelope.PumpDump = signal.StepTrade(envelope.TradeData)
	case types.EnvelopeLevel3:
		if envelope.Level3Data.Bids == nil && envelope.Level3Data.Asks == nil {
			return envelope
		}

		envelope.PumpDump = signal.StepLevel3(envelope.Level3Data)
	}

	return envelope
}

func (signal *Signal) StepTicker(ticker kraken.TickerData) *data.Measurement[float64] {
	return signal.ticker.Step(ticker)
}

func (signal *Signal) StepTrade(trade kraken.TradeData) *data.Measurement[float64] {
	return signal.trade.Step(trade)
}

func (signal *Signal) StepLevel3(message kraken.Level3Data) *data.Measurement[float64] {
	return signal.level3.Step(message)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if err := signal.ticker.Close(); err != nil {
		return err
	}

	if err := signal.trade.Close(); err != nil {
		return err
	}

	return signal.level3.Close()
}

type tickerState struct {
	graph core.Primitive
}

/*
Ticker is the executable-touch market entity. It measures displayed capacity,
executable spread, and the historical relative-spread baseline through one
causal Baseline primitive per symbol.
*/
type Ticker struct {
	states map[string]*tickerState
}

func NewTicker() *Ticker {
	return &Ticker{
		states: make(map[string]*tickerState),
	}
}

func (ticker *Ticker) Close() error {
	return nil
}

func (ticker *Ticker) Step(tick kraken.TickerData) *data.Measurement[float64] {
	if tick.Bid == nil || tick.Ask == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("pumpdump: ticker requires bid and ask")}
	}

	bid := tick.Bid.Float64()
	ask := tick.Ask.Float64()

	if bid <= 0 || ask <= 0 || ask <= bid {
		return &data.Measurement[float64]{Err: fmt.Errorf("pumpdump: positive order violated (%f <= %f)", ask, bid)}
	}

	midpoint := (bid + ask) / 2.0
	spread := ask - bid
	relativeSpread := spread / midpoint

	state, found := ticker.states[tick.Symbol]

	if !found {
		state = &tickerState{graph: adaptive.NewBaseline(adaptive.NewWindow())}
		ticker.states[tick.Symbol] = state
	}

	readingEval := transport.NewEvaluate(state.graph)
	var reading adaptive.BaselineReading

	for out := range readingEval.Next(transport.NewValues(relativeSpread).Next(nil)) {
		reading = *(*adaptive.BaselineReading)(out)
	}

	if err := readingEval.Error(); err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	id := fmt.Sprintf("pumpdump:%s:%d", tick.Symbol, tick.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64]("pumpdump", nil)
	measurement.Label, measurement.At, measurement.From = tick.Symbol, tick.Timestamp, tick.Timestamp
	measurement.Metadata = make(map[string]float64)

	putPumpDumpMetric(measurement, "best_bid", bid, data.UnitRate)
	putPumpDumpMetric(measurement, "best_ask", ask, data.UnitRate)
	putPumpDumpMetric(measurement, "midpoint", midpoint, data.UnitRate)
	putPumpDumpMetric(measurement, "spread", spread, data.UnitRate)
	putPumpDumpMetric(measurement, "relative_spread", relativeSpread, data.UnitDimensionless)
	putPumpDumpMetric(measurement, "relative_spread_baseline", reading.Baseline, data.UnitDimensionless)
	putPumpDumpMetric(measurement, "spread_ratio", relativeSpread/reading.Baseline, data.UnitDimensionless)

	// Quality is derived by Finalize from the measurement's own facts, never
	// assigned here. spread_zscore is this entity's headline reading, so its
	// estimator supplies the support, the departure, and the noise power the
	// SNR is derived from.
	measurement.Metadata[data.MetadataSupport] = reading.Prior.Count

	if reading.HasPrior {
		putPumpDumpMetric(measurement, "spread_divergence", reading.Residual, data.UnitDimensionless)
		putPumpDumpMetric(measurement, "spread_zscore", reading.ZScore, data.UnitDimensionless)

		if reading.PriorVariance > 0 {
			measurement.Metadata[data.MetadataDivergence] = reading.Residual
			measurement.Metadata[data.MetadataNoiseVariance] = reading.PriorVariance
		}
	}

	measurement.Finalize()

	return measurement
}

func putPumpDumpMetric(m *data.Measurement[float64], name string, val float64, unit data.Unit) {
	m.Metrics[name] = data.Metric[float64]{
		Label: name, Raw: val, Unit: unit, Timescale: data.TimescaleInstantaneous,
	}
}
