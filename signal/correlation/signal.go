package correlation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/theapemachine/errnie"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	tickerIn chan []kraken.TickerData
	bookIn   chan []kraken.BookData
	tradeIn  chan []kraken.TradeData
	ctx      context.Context
	cancel   context.CancelFunc
	section  *Section
	ui       chan []byte
}

/*
NewSignal creates correlation measurement state for central market cuts so
successive ticks can establish real price relationships.
*/
func NewSignal(ctx context.Context, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		tickerIn: make(chan []kraken.TickerData, 64),
		bookIn:   make(chan []kraken.BookData, 64),
		tradeIn:  make(chan []kraken.TradeData, 64),
		ctx:      ctx,
		cancel:   cancel,
		section:  NewSection(),
		ui:       ui,
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
	crossSection := types.NewCrossSection()
	if len(tickers) > 0 {
		crossSection.Measure(tickers)
	}

	if crossSection == nil {
		return nil, fmt.Errorf("correlation: cross section required")
	}

	rows := tickers
	out := make([]*types.Measurement, 0, len(rows))

	scoresBySymbol, err := signal.section.Measure(rows)

	if err != nil {
		return nil, err
	}
	latestAtBySymbol := make(map[string]time.Time, len(rows))

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if !row.Timestamp.After(latestAtBySymbol[symbol]) {
			continue
		}

		latestAtBySymbol[symbol] = row.Timestamp
	}

	crossSection.Metrics.Range(func(_, value any) bool {
		metric := value.(types.SymbolMetric)
		scores, ok := scoresBySymbol[metric.Symbol]

		if !ok {
			return true
		}

		at, ok := latestAtBySymbol[metric.Symbol]

		if !ok || at.IsZero() {
			return true
		}

		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}

		measurements := []*types.Measurement{
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricCorrelation,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["correlation"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricSigned,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["signed"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricRelativeEnergy,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["relativeEnergy"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricHerdScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["herdScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricAlphaScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["alphaScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricNoiseScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["noiseScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricStressScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["stressScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricPeakScore,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["peakScore"],
				Validity: validity,
			},
			{
				Source:   types.SourceCorrelation,
				Metric:   types.MetricStrength,
				Stream:   types.Correlation,
				Symbol:   metric.Symbol,
				At:       at,
				Unit:     types.UnitDimensionless,
				Raw:      scores["strength"],
				Validity: validity,
			},
		}

		out = append(out, measurements...)
		return true
	})

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
					continue
				}

				if len(measured) == 0 {
					continue
				}

				select {
				case out <- measured:
					signal.Publish(measured)
				default:
				}
			case rows := <-signal.bookIn:
				measured, err := signal.Calculate(nil, nil, rows)

				if err != nil {
					errnie.Error(err)
					continue
				}

				if len(measured) == 0 {
					continue
				}

				select {
				case out <- measured:
					signal.Publish(measured)
				default:
				}
			case rows := <-signal.tradeIn:
				measured, err := signal.Calculate(nil, rows, nil)

				if err != nil {
					errnie.Error(err)
					continue
				}

				if len(measured) == 0 {
					continue
				}

				select {
				case out <- measured:
					signal.Publish(measured)
				default:
				}
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()

	return out
}
