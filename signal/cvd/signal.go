package cvd

import (
	"context"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the Absorption perspective, measuring signed aggressor flow against
price response. Categories belong in logic; this signal emits numerical scores
only.
*/
type Signal struct {
	tickerIn  chan []kraken.TickerData
	bookIn    chan []kraken.BookData
	tradeIn   chan []kraken.TradeData
	ack     chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	sample    *algorithm.TradeFlowSample
	flow      *equation.Flow
	midpoints map[string]float64
	ui        chan []byte
}

/*
NewSignal creates the CVD perspective with independent rolling state for each
symbol so one market's aggressor history cannot leak into another's evidence.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		tickerIn:  make(chan []kraken.TickerData, 64),
		bookIn:    make(chan []kraken.BookData, 64),
		tradeIn:   make(chan []kraken.TradeData, 64),
		ack:     make(chan struct{}, 256),
		ctx:       ctx,
		cancel:    cancel,
		sample:    algorithm.NewTradeFlowSample(),
		flow:      equation.NewFlow(),
		midpoints: make(map[string]float64),
		ui:        ui,
	}

	return signal
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
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	out := make([]*types.Measurement, 0, len(trades))

	for _, row := range tickers {
		if row.Symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if row.Bid == nil || row.Ask == nil {
			continue
		}

		if row.Bid.Sign() <= 0 || row.Ask.Cmp(row.Bid) <= 0 {
			continue
		}

		signal.midpoints[row.Symbol] = (row.Bid.Float64() + row.Ask.Float64()) / 2
	}

	for _, row := range trades {
		if row.Symbol == "" || row.Price.Sign() <= 0 || row.Qty <= 0 {
			continue
		}

		midpoint, exists := signal.midpoints[row.Symbol]

		if !exists {
			continue
		}

		measurements, err := signal.measureTrade(row, midpoint)

		if err != nil {
			continue
		}

		out = append(out, measurements...)
	}

	return out, nil
}

/*
measureTrade separates execution notional from midpoint response before
classifying one aggressor observation through the adaptive CVD window.
*/
func (signal *Signal) measureTrade(
	row kraken.TradeData,
	midpoint float64,
) ([]*types.Measurement, error) {
	input, ready, maturity, err := signal.sample.Measure(algorithm.TradeFlowInput{
		Symbol:        row.Symbol,
		Price:         row.Price.Float64(),
		ResponsePrice: midpoint,
		Quantity:      row.Qty,
		Side:          row.Side,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := signal.flow.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	return signal.cvdMeasurements(row, output, maturity), nil
}

/*
cvdMeasurements maps signed-flow output into typed CVD measurements while
preserving each metric's unit and normalization.
*/
func (signal *Signal) cvdMeasurements(
	row kraken.TradeData,
	output equation.FlowOutput,
	maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	scale := types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    row.Timestamp,
		Through: row.Timestamp,
	}
	specs := []struct {
		metric types.MetricType
		unit   types.MeasurementUnit
		value  float64
	}{
		{types.MetricAbsorption, types.UnitDimensionless, output.Absorption},
		{types.MetricDrive, types.UnitDimensionless, output.Drive},
		{types.MetricBalance, types.UnitDimensionless, output.Balance},
		{types.MetricStarvation, types.UnitDimensionless, output.Starvation},
		{types.MetricStrength, types.UnitDimensionless, output.Value},
		{types.MetricNetFraction, types.UnitDimensionless, output.NetFraction},
		{types.MetricNet, types.UnitQuoteCurrency, output.Net},
	}
	measurements := make([]*types.Measurement, 0, len(specs))

	for _, spec := range specs {
		var normalized *float64

		if spec.unit == types.UnitDimensionless {
			normalized = types.NormalizeFinite(spec.value)
		}

		measurements = append(measurements, &types.Measurement{
			Source:     types.SourceCVD,
			Stream:     types.CVD,
			Metric:     spec.metric,
			Subject:    types.SubjectAggressorFlow,
			Symbol:     row.Symbol,
			At:         row.Timestamp,
			Unit:       spec.unit,
			Raw:        spec.value,
			Normalized: normalized,
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		})
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}

/*
Tickers returns the ticker ingress channel.
*/
func (signal *Signal) Tickers() chan []kraken.TickerData {
	return signal.tickerIn
}

/*
Books returns the book ingress channel.
*/
func (signal *Signal) Books() chan []kraken.BookData {
	return signal.bookIn
}

/*
Trades returns the trade ingress channel.
*/
func (signal *Signal) Trades() chan []kraken.TradeData {
	return signal.tradeIn
}


/*
Ack signals that one ingress frame finished Calculate so Crypto can barrier
before draining outs.
*/
func (signal *Signal) Ack() <-chan struct{} {
	return signal.ack
}

/*
Measure consumes ingress channels and sends measurements on out.
*/
func (signal *Signal) Measure() chan []*types.Measurement {
	out := make(chan []*types.Measurement, 64)

	go func() {
		defer close(out)

		for {
			select {
			case <-signal.ctx.Done():
				return
			case rows := <-signal.tickerIn:
				measured, err := signal.Calculate(rows, nil, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.bookIn:
				measured, err := signal.Calculate(nil, nil, rows)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			case rows := <-signal.tradeIn:
				measured, err := signal.Calculate(nil, rows, nil)

				if err != nil {
					errnie.Error(err)
					signal.ack <- struct{}{}
					continue
				}

				if len(measured) == 0 {
					signal.ack <- struct{}{}
					continue
				}

				out <- measured
				signal.Publish(measured)
				signal.ack <- struct{}{}
			}
		}
	}()

	return out
}
