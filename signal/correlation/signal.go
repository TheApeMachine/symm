package correlation

import (
	"context"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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

	signal.Actor = types.NewActor(ctx, map[string]types.Handler{
		"ticker": {Topic: "thesis", Fn: signal.onTicker},
		"book":   {Topic: "thesis", Fn: signal.onBook},
		"trade":  {Topic: "thesis", Fn: signal.onTrade},
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
Initialize wires ticker, book, and trade ingress from Live.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis
	signal.Actor.Initialize(
		types.Topic{Name: "ticker", Actor: live},
		types.Topic{Name: "book", Actor: live},
		types.Topic{Name: "trade", Actor: live},
	)
}


func (signal *Signal) onTicker(message any) any {
	rows := message.(*kraken.Ticker).Data
	measurements, err := signal.Calculate(rows, nil, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Measurements = append(signal.thesis.Measurements, measurements...)

	return signal.thesis
}

func (signal *Signal) onBook(message any) any {
	rows := message.(*kraken.Book).Data
	measurements, err := signal.Calculate(nil, nil, rows)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Measurements = append(signal.thesis.Measurements, measurements...)

	return signal.thesis
}

func (signal *Signal) onTrade(message any) any {
	rows := message.(*kraken.Trade).Data
	measurements, err := signal.Calculate(nil, rows, nil)

	if err != nil {
		errnie.Error(err)
		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	signal.thesis.Measurements = append(signal.thesis.Measurements, measurements...)

	return signal.thesis
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []kraken.BookData,
) ([]*types.Measurement, error) {
	if len(tickers) == 0 {
		return nil, nil
	}

	scoresBySymbol, err := signal.section.Measure(tickers)

	if err != nil || len(scoresBySymbol) == 0 {
		return nil, err
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

	out := make([]*types.Measurement, 0, len(scoresBySymbol)*9)
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	for symbol, scores := range scoresBySymbol {
		at, ok := latestAtBySymbol[symbol]

		if !ok || at.IsZero() {
			continue
		}

		out = appendCorrelation(
			out, symbol, at, validity, scores,
		)
	}

	return out, nil
}

/*
appendCorrelation writes the nine cohort evidence rows for one symbol.
*/
func appendCorrelation(
	out []*types.Measurement,
	symbol string,
	at time.Time,
	validity types.MeasurementValidity,
	scores map[string]float64,
) []*types.Measurement {
	specs := []struct {
		metric types.MetricType
		key    string
	}{
		{types.MetricCorrelation, "correlation"},
		{types.MetricSigned, "signed"},
		{types.MetricRelativeEnergy, "relativeEnergy"},
		{types.MetricHerdScore, "herdScore"},
		{types.MetricAlphaScore, "alphaScore"},
		{types.MetricNoiseScore, "noiseScore"},
		{types.MetricStressScore, "stressScore"},
		{types.MetricPeakScore, "peakScore"},
		{types.MetricStrength, "strength"},
	}

	for _, spec := range specs {
		out = append(out, &types.Measurement{
			Source:   types.SourceCorrelation,
			Metric:   spec.metric,
			Stream:   types.Correlation,
			Symbol:   symbol,
			At:       at,
			Unit:     types.UnitDimensionless,
			Raw:      scores[spec.key],
			Validity: validity,
		})
	}

	return out
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
