package derivatives

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/types"
)

/*
Signal is the derivatives measuring instrument. It composes its market
entities in its constructor and exposes the canonical signal structure:
Constructor, Name, Error, Step, Close. It satisfies
nomagique/runtime.Node[*types.Envelope], dispatching on the envelope's
TypeID to whichever entity that futures data stream feeds — mirrors
signal/pumpdump's dispatch shape, but over the futures ticker/trade
envelope kinds rather than spot ones.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	ticker *Ticker
	trade  *Trade
}

/*
NewSignal composes the Ticker (derivative/reference basis and open interest)
and Trade (liquidation accounting) entities.
*/
func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		ticker: NewTicker(),
		trade:  NewTrade(),
	}
}

func (signal *Signal) Name() string { return "derivatives" }

func (signal *Signal) Error() error { return signal.err }

func (signal *Signal) Step(envelope *types.Envelope) *types.Envelope {
	if signal.err != nil {
		errnie.Error(signal.Close())
		return nil
	}

	switch envelope.TypeID {
	case types.EnvelopeFuturesTicker:
		envelope.Derivatives = signal.StepTicker(envelope.FuturesTickerData)
	case types.EnvelopeFuturesTrade:
		envelope.Derivatives = signal.StepTrade(envelope.FuturesTradeData)
	}

	return envelope
}

func (signal *Signal) StepTicker(ticker kraken.FuturesTickerData) *data.Measurement[float64] {
	return signal.ticker.Step(ticker)
}

func (signal *Signal) StepTrade(trade kraken.FuturesTradeData) *data.Measurement[float64] {
	return signal.trade.Step(trade)
}

func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if err := signal.ticker.Close(); err != nil {
		return err
	}

	return signal.trade.Close()
}

type tickerState struct {
	basisGraph  core.Primitive
	growthGraph core.Primitive

	hasPrev   bool
	prevTime  time.Time
	prevLast  float64
	prevIndex float64
	prevSpot  float64
	prevOI    float64
	prevBasis float64

	prevBasisRate    float64
	hasPrevBasisRate bool
	prevReturnGap    float64
	hasPrevReturnGap bool
}

/*
Ticker is the derivative/reference state market entity. It measures basis,
open-interest growth, and cross-instrument relative returns using native nomagique
v2 equations without Frame or Wire blocks.
*/
type Ticker struct {
	states map[string]*tickerState
	clock  causalClock
}

func NewTicker() *Ticker {
	return &Ticker{
		states: make(map[string]*tickerState),
		clock:  newCausalClock(),
	}
}

func (ticker *Ticker) Close() error {
	return nil
}

func (ticker *Ticker) Step(point kraken.FuturesTickerData) *data.Measurement[float64] {
	if point.Last == nil || point.IndexPrice == nil || point.MarkPrice == nil {
		return &data.Measurement[float64]{Err: fmt.Errorf("derivatives: last, index, and mark prices required")}
	}
	stamped, advanced := ticker.clock.stamp(
		point.Symbol, point.Timestamp, point.SyntheticTimestamp,
	)
	point.Timestamp = stamped

	last := point.Last.Float64()
	index := point.IndexPrice.Float64()
	mark := point.MarkPrice.Float64()
	oi := point.OpenInterest

	if index <= 0 || mark <= 0 || last < 0 || oi < 0 {
		return &data.Measurement[float64]{
			Err: fmt.Errorf("derivatives: non-positive prices or oi (last=%f, index=%f, mark=%f, oi=%f)", last, index, mark, oi),
		}
	}

	state, found := ticker.states[point.Symbol]

	if !found {
		state = &tickerState{
			basisGraph:  adaptive.NewBaseline(adaptive.NewWindow()),
			growthGraph: adaptive.NewBaseline(adaptive.NewWindow()),
		}
		ticker.states[point.Symbol] = state
	}

	basis := (last - index) / index

	basisReadingEval := transport.NewEvaluate(state.basisGraph)
	var basisReading adaptive.BaselineReading

	for out := range basisReadingEval.Next(transport.NewValues(basis).Next(nil)) {
		basisReading = *(*adaptive.BaselineReading)(out)
	}

	err := basisReadingEval.Error()
	if err != nil {
		return &data.Measurement[float64]{Err: err}
	}

	id := fmt.Sprintf("derivatives:%s:%d", point.Symbol, point.Timestamp.UnixNano())
	measurement := data.NewMeasurement[float64]("derivatives", nil)
	measurement.Label, measurement.At, measurement.From = point.Symbol, point.Timestamp, point.Timestamp
	measurement.Metadata = make(map[string]float64)

	putDerivMetric(measurement, "derivative_price", last, data.UnitRate)
	putDerivMetric(measurement, "reference_price", index, data.UnitRate)
	putDerivMetric(measurement, "spot_price", mark, data.UnitRate)
	putDerivMetric(measurement, "open_interest", oi, data.UnitCount)
	putDerivMetric(measurement, "basis", basis, data.UnitDimensionless)

	putDerivMetric(measurement, "basis_baseline", basisReading.Baseline, data.UnitDimensionless)

	if basisReading.HasPrior {
		putDerivMetric(measurement, "basis_zscore", basisReading.ZScore, data.UnitDimensionless)
	}

	if last > 0 && index > 0 {
		logBasis := math.Log(last / index)
		putDerivMetric(measurement, "log_basis", logBasis, data.UnitDimensionless)

		if mark > 0 {
			derivIndexBasis := math.Log(last / index)
			indexSpotBasis := math.Log(index / mark)
			derivSpotBasis := math.Log(last / mark)

			putDerivMetric(measurement, "derivative_index_log_basis", derivIndexBasis, data.UnitDimensionless)
			putDerivMetric(measurement, "index_spot_log_basis", indexSpotBasis, data.UnitDimensionless)
			putDerivMetric(measurement, "derivative_spot_log_basis", derivSpotBasis, data.UnitDimensionless)
			putDerivMetric(measurement, "basis_closure_error", 0.0, data.UnitDimensionless)
		}
	}

	if advanced && state.hasPrev {
		dt := stamped.Sub(state.prevTime).Seconds()

		oiChange := oi - state.prevOI
		putDerivMetric(measurement, "open_interest_change", oiChange, data.UnitCount)

		if state.prevOI > 0 && oi > 0 {
			oiLogChange := math.Log(oi / state.prevOI)
			putDerivMetric(measurement, "open_interest_log_change", oiLogChange, data.UnitDimensionless)

			if dt > 0 {
				oiGrowthRate := oiLogChange / dt
				putDerivMetric(measurement, "open_interest_growth_rate", oiGrowthRate, data.UnitPerSecond)

				growthEval := transport.NewEvaluate(state.growthGraph)
				var growth adaptive.BaselineReading

				for out := range growthEval.Next(transport.NewValues(oiGrowthRate).Next(nil)) {
					growth = *(*adaptive.BaselineReading)(out)
				}

				err := growthEval.Error()
				if err != nil {
					measurement.Err = err
					return measurement
				}
				putDerivMetric(measurement, "open_interest_growth_baseline", growth.Baseline, data.UnitPerSecond)
			}
		}

		if dt > 0 {
			basisChange := basis - state.prevBasis
			basisRate := basisChange / dt
			putDerivMetric(measurement, "basis_change", basisChange, data.UnitDimensionless)
			putDerivMetric(measurement, "basis_rate", basisRate, data.UnitPerSecond)

			// Velocity is the change in the RATE between consecutive
			// observations: basis_rate says how fast the basis is moving,
			// basis_velocity says whether that movement is accelerating.
			if state.hasPrevBasisRate {
				putDerivMetric(
					measurement, "basis_velocity",
					basisRate-state.prevBasisRate, data.UnitPerSecond,
				)
			}

			state.prevBasisRate = basisRate
			state.hasPrevBasisRate = true
		}

		var hasDerivReturn, hasRefReturn bool
		var derivLogReturn, refLogReturn float64

		if state.prevLast > 0 && last > 0 {
			derivLogReturn = math.Log(last / state.prevLast)
			putDerivMetric(measurement, "derivative_log_return", derivLogReturn, data.UnitDimensionless)
			hasDerivReturn = true
		}

		if state.prevIndex > 0 && index > 0 {
			refLogReturn = math.Log(index / state.prevIndex)
			putDerivMetric(measurement, "reference_log_return", refLogReturn, data.UnitDimensionless)
			hasRefReturn = true
		}

		if hasDerivReturn && hasRefReturn {
			returnGap := derivLogReturn - refLogReturn
			putDerivMetric(measurement, "return_gap", returnGap, data.UnitDimensionless)

			if state.hasPrevReturnGap {
				putDerivMetric(
					measurement, "return_gap_velocity",
					returnGap-state.prevReturnGap, data.UnitDimensionless,
				)
			}

			state.prevReturnGap = returnGap
			state.hasPrevReturnGap = true
		}
	}

	if advanced {
		state.prevTime = stamped
		state.prevLast = last
		state.prevIndex = index
		state.prevSpot = mark
		state.prevOI = oi
		state.prevBasis = basis
		state.hasPrev = true
	}

	// Quality is derived by Finalize from the measurement's own facts, never
	// assigned here. The basis z-score is scored against the moments held
	// BEFORE this observation, so the evidence backing it is the prior count —
	// one prior sample carries no dispersion of its own and is immature.
	// A measurement with no estimator behind it is a whole direct reading and
	// declares no support at all.
	if basisReading.HasPrior {
		measurement.Metadata[data.MetadataSupport] = basisReading.Prior.Count

		// basis_zscore is this entity's headline reading, so its estimator
		// supplies the departure and the noise power Finalize turns into SNR.
		// Without them the measurement projected its metrics but reported no
		// SNR at all, which reads downstream as a kernel that never measured.
		if basisReading.PriorVariance > 0 {
			measurement.Metadata[data.MetadataDivergence] = basisReading.Residual
			measurement.Metadata[data.MetadataNoiseVariance] = basisReading.PriorVariance
		}
	}

	measurement.Finalize()

	return measurement
}
