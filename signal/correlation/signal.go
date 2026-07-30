package correlation

import (
	"context"
	"strings"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it. Categories belong in logic; this signal
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
NewSignal creates correlation measurement state for central market cuts so
successive ticks can establish real price relationships.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:     ctx,
		cancel:  cancel,
		section: NewSection(),
		ui:      ui,
	}

	signal.Actor = types.NewActor(ctx, "correlation", map[string]types.Handler{
		"ticker": {Topic: "thesis", Fn: signal.onTicker},
	})

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceCorrelation)
}

/*
Initialize wires ticker ingress from Live. Correlation is ticker-cross-section
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
	return signal.thesis.AppendMeasuremnts(
		types.SourceCorrelation,
		signal.Calculate(rows, nil, nil),
	)
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) []*types.Measurement {
	if len(tickers) == 0 {
		return nil
	}

	scoresBySymbol, err := signal.section.Measure(tickers)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "correlation: failed to measure tickers", err,
		))

		return nil
	}

	if len(scoresBySymbol) == 0 {
		return nil
	}

	latestAtBySymbol := make(map[string]time.Time, len(tickers))

	for _, row := range tickers {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if !row.Timestamp.After(latestAtBySymbol[symbol]) {
			continue
		}

		latestAtBySymbol[symbol] = row.Timestamp
	}

	out := make([]*types.Measurement, 0, len(scoresBySymbol))
	uiOut := datura.NewMap(
		"measurements", make([]*types.Measurement, 0),
	)

	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	for symbol, scores := range scoresBySymbol {
		at := latestAtBySymbol[symbol]

		if at.IsZero() {
			at = signal.section.LastAt(symbol)
		}

		if at.IsZero() {
			continue
		}

		measurement := correlationMeasurement(symbol, at, validity, scores)
		out = append(out, measurement)

		if measurement.Symbol == types.Focus() {
			uiOut["measurements"] = append(
				uiOut["measurements"].([]*types.Measurement), measurement,
			)
		}
	}

	if len(uiOut["measurements"].([]*types.Measurement)) > 0 {
		utils.Publish(signal.ui, uiOut)
	}

	return out
}

/*
correlationMeasurement writes the nine cohort evidence metrics for one symbol.
*/
func correlationMeasurement(
	symbol string,
	at time.Time,
	validity types.MeasurementValidity,
	scores map[string]float64,
) *types.Measurement {
	measurement := &types.Measurement{
		Source:   types.SourceCorrelation,
		Symbol:   symbol,
		At:       at,
		Validity: validity,
		Metrics:  make(map[string]types.MetricSample, 9),
	}

	measurement.Metrics[types.MetricKey(types.MetricCorrelation, types.SideNone)] = types.MetricSample{Raw: scores["correlation"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricSigned, types.SideNone)] = types.MetricSample{Raw: scores["signed"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricRelativeEnergy, types.SideNone)] = types.MetricSample{Raw: scores["relativeEnergy"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricHerdScore, types.SideNone)] = types.MetricSample{Raw: scores["herdScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricAlphaScore, types.SideNone)] = types.MetricSample{Raw: scores["alphaScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricNoiseScore, types.SideNone)] = types.MetricSample{Raw: scores["noiseScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStressScore, types.SideNone)] = types.MetricSample{Raw: scores["stressScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricPeakScore, types.SideNone)] = types.MetricSample{Raw: scores["peakScore"], Unit: types.UnitDimensionless}
	measurement.Metrics[types.MetricKey(types.MetricStrength, types.SideNone)] = types.MetricSample{Raw: scores["strength"], Unit: types.UnitDimensionless}

	return measurement
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
